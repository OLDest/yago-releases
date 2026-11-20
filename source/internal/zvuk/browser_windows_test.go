package zvuk

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDebuggerLoopbackOnly(t *testing.T) {
	for _, s := range []string{"ws://remote.example/devtools/browser/x", "ws://127.0.0.1:9/devtools/x?token=secret", "wss://127.0.0.1:9/x", "ws://u@127.0.0.1:9/x"} {
		if validDebugger(s) {
			t.Fatal(s)
		}
	}
	if !validDebugger("ws://127.0.0.1:1234/devtools/browser/x") {
		t.Fatal("loopback rejected")
	}
}
func TestBrowserCancellation(t *testing.T) {
	if os.Getenv("YAGO_BROWSER_TEST") != "1" {
		t.Skip("explicit interactive browser check")
	}
	if browserExecutable() == "" {
		t.Fatal("no installed browser")
	}
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "yago-zvuk-login-*"))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	seen := false
	_, err := browserLoginObserved(ctx, func(state browserWindowState) {
		seen = true
		t.Logf("Login window: state=%s, bounds=%dx%d", state.WindowState, state.Width, state.Height)
	})
	if err == nil {
		t.Fatal("unexpected login")
	}
	if ctx.Err() == nil {
		t.Fatal("browser failed before cancellation:", err)
	}
	if !seen {
		t.Fatal("login window did not expose visible bounds before cancellation")
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "yago-zvuk-login-*"))
	if len(after) != len(before) {
		t.Fatal("temporary browser profile leaked", len(before), len(after))
	}
	t.Log("Installed browser opened; cancellation closed session and removed its profile")
}
