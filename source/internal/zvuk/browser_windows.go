package zvuk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/net/websocket"
	"golang.org/x/sys/windows"
	"yago/internal/music"
)

// BrowserLogin observes only the temporary profile it owns. Existing browser sessions are untouched.
func BrowserLogin(parent context.Context) (LoginResult, error) {
	return browserLoginObserved(parent, nil)
}

type browserWindowState struct {
	WindowState   string
	Width, Height int
}

func browserLoginObserved(parent context.Context, observe func(browserWindowState)) (LoginResult, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	exe := browserExecutable()
	if exe == "" {
		return LoginResult{}, errors.New("Для входа в Звук нужен установленный Microsoft Edge или Google Chrome")
	}
	dir, e := os.MkdirTemp("", "yago-zvuk-login-")
	if e != nil {
		return LoginResult{}, errors.New("Не удалось создать окно входа")
	}
	// Only this exact newly created directory is ever removed.
	defer func() {
		if filepath.Dir(dir) == os.TempDir() && strings.HasPrefix(filepath.Base(dir), "yago-zvuk-login-") {
			deadline := time.Now().Add(2 * time.Second)
			for {
				if os.RemoveAll(dir) == nil || time.Now().After(deadline) {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return LoginResult{}, errors.New("Не удалось создать сессию входа")
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		return LoginResult{}, errors.New("Не удалось настроить завершение окна входа")
	}
	process, e := startBrowserProcess(exe, []string{"--user-data-dir=" + dir, "--guest", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0", "--remote-allow-origins=http://127.0.0.1", "--no-first-run", "--no-default-browser-check", "--disable-background-mode", "--app=" + loginURL}, job)
	if e != nil {
		return LoginResult{}, fmt.Errorf("Не удалось открыть браузер входа: %w", e)
	}
	defer stopBrowserJob(job, process)
	timer := time.NewTicker(400 * time.Millisecond)
	defer timer.Stop()
	var endpoint string
	for endpoint == "" {
		select {
		case <-ctx.Done():
			return LoginResult{}, loginCanceled(ctx)
		case <-timer.C:
		}
		if active, err := browserJobActive(job); err != nil {
			return LoginResult{}, err
		} else if !active {
			return LoginResult{}, browserStartupExit(process)
		}
		b, _ := os.ReadFile(filepath.Join(dir, "DevToolsActivePort"))
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) != 2 {
			continue
		}
		port, e := strconv.Atoi(strings.TrimSpace(lines[0]))
		path := strings.TrimSpace(lines[1])
		if e == nil && port > 0 && port < 65536 && strings.HasPrefix(path, "/devtools/browser/") && !strings.ContainsAny(path, "?# \r") {
			endpoint = fmt.Sprintf("ws://127.0.0.1:%d%s", port, path)
		}
	}
	config, e := websocket.NewConfig(endpoint, "http://127.0.0.1")
	if e != nil || !validDebugger(endpoint) {
		return LoginResult{}, errors.New("Не удалось подключить браузер")
	}
	ws, e := config.DialContext(ctx)
	if e != nil {
		return LoginResult{}, errors.New("Браузер не разрешил подключение")
	}
	defer ws.Close()
	ws.MaxPayloadBytes = 128 << 10
	stop := context.AfterFunc(ctx, func() { ws.Close() })
	defer stop()
	seq := 0
	call := func(method string, params any, session string, out any) error {
		seq++
		_ = ws.SetDeadline(time.Now().Add(8 * time.Second))
		message := map[string]any{"id": seq, "method": method, "params": params}
		if session != "" {
			message["sessionId"] = session
		}
		if e := websocket.JSON.Send(ws, message); e != nil {
			return errors.New("Окно входа недоступно")
		}
		for {
			var msg struct {
				ID     int
				Result json.RawMessage
				Error  json.RawMessage
			}
			if e := websocket.JSON.Receive(ws, &msg); e != nil {
				return errors.New("Окно входа закрыто")
			}
			if msg.ID != seq {
				continue
			}
			if len(msg.Error) > 0 {
				return errors.New("Браузер отклонил запрос входа")
			}
			if out != nil {
				return json.Unmarshal(msg.Result, out)
			}
			return nil
		}
	}

	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return LoginResult{}, loginCanceled(ctx)
		case <-poll.C:
		}
		if active, err := browserJobActive(job); err != nil {
			return LoginResult{}, err
		} else if !active {
			return LoginResult{}, errors.New("Окно входа закрыто")
		}
		var targets struct {
			Items []struct {
				ID        string `json:"targetId"`
				Type, URL string
				ContextID string `json:"browserContextId"`
			} `json:"targetInfos"`
		}
		if e = call("Target.getTargets", map[string]any{}, "", &targets); e != nil {
			return LoginResult{}, e
		}
		for _, target := range targets.Items {
			u, err := url.Parse(target.URL)
			if observe != nil && err == nil && target.Type == "page" && u.Scheme == "https" && (u.Host == "id.zvuk.com" || u.Host == "zvuk.com") {
				var window struct{ Bounds browserWindowState }
				if call("Browser.getWindowForTarget", map[string]any{"targetId": target.ID}, "", &window) == nil && window.Bounds.Width > 0 && window.Bounds.Height > 0 && window.Bounds.WindowState != "minimized" {
					observe(window.Bounds)
					observe = nil
				}
			}
			if err != nil || target.Type != "page" || u.Scheme != "https" || u.Host != "zvuk.com" {
				continue
			}
			var attach struct {
				ID string `json:"sessionId"`
			}
			if e = call("Target.attachToTarget", map[string]any{"targetId": target.ID, "flatten": true}, "", &attach); e != nil {
				continue
			}
			var result struct {
				Result    struct{ Value string }
				Exception json.RawMessage `json:"exceptionDetails"`
			}
			err = call("Runtime.evaluate", map[string]any{"expression": profileExpression, "awaitPromise": true, "returnByValue": true}, attach.ID, &result)
			_ = call("Target.detachFromTarget", map[string]any{"sessionId": attach.ID}, "", nil)
			if err != nil || len(result.Exception) > 0 || result.Result.Value == "" {
				continue
			}
			profile, err := ParseProfile([]byte(result.Result.Value))
			if err != nil {
				continue
			}
			token := profile.Token
			if token == "" {
				var cookies struct {
					Cookies []struct{ Name, Value, Domain string }
				}
				params := map[string]any{}
				if target.ContextID != "" {
					params["browserContextId"] = target.ContextID
				}
				if e = call("Storage.getCookies", params, "", &cookies); e != nil {
					continue
				}
				for _, cookie := range cookies.Cookies {
					if cookie.Name == "auth" && (cookie.Domain == "zvuk.com" || cookie.Domain == ".zvuk.com") {
						token = cookie.Value
						break
					}
				}
			}
			if token == "" {
				continue
			}
			name := profile.Name
			if name == "" {
				name = "Звук"
			}
			_ = call("Browser.close", map[string]any{}, "", nil)
			return LoginResult{Token: token, Account: music.Account{UID: ns(profile.ID), DisplayName: name, Login: "zvuk"}}, nil
		}
	}

}
func loginCanceled(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("Время входа в Звук истекло")
	}
	return errors.New("Вход в Звук отменён")
}
func browserExecutable() string {
	for _, root := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		if root == "" {
			continue
		}
		for _, rel := range []string{`Microsoft\Edge\Application\msedge.exe`, `Google\Chrome\Application\chrome.exe`} {
			p := filepath.Join(root, rel)
			if info, e := os.Stat(p); e == nil && !info.IsDir() {
				return p
			}
		}
	}
	return ""
}

// Reject an arbitrary debugger endpoint: the endpoint must come from our new profile, on loopback.
func validDebugger(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "ws" && u.Hostname() == "127.0.0.1" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

const profileExpression = `(async()=>{
 if(location.origin!=="https://zvuk.com")return "";
 for(const path of ["/api/v2/tiny/profile","/api/tiny/profile"]){
  try{
   const r=await fetch(path,{credentials:"include",signal:AbortSignal.timeout(2500)});
   if(!r.ok)continue;
   const text=await r.text();if(text.length>65536)continue;
   const j=JSON.parse(text),p=j.result?.profile??j.result;
   if(p?.id!=null&&p.is_anonymous===false)return text;
  }catch{}
 }
 return "";
})()`
