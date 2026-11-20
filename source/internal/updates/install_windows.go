package updates

import (
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type installJob struct {
	Target, Staged, Manifest, State, Marker string
	PID                                     uint32
}

func copyFile(src, dst string) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	if e == nil {
		e = out.Sync()
	}
	ce := out.Close()
	if e != nil {
		return e
	}
	return ce
}
func hidden(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
func Install(path, state string) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	exe, e = filepath.EvalSymlinks(exe)
	if e != nil {
		return e
	}
	raw, e := os.ReadFile(filepath.Join(filepath.Dir(path), "updates.json"))
	if e != nil {
		return e
	}
	m, e := Verify(raw)
	if e != nil {
		return e
	}
	a, ok := m.Artifacts["windows_amd64"]
	if !ok || !Newer(m.Version, Version) {
		return errors.New("Обновление устарело")
	}
	if e = VerifyFile(path, a); e != nil {
		return e
	}
	// Check destination access before closing the application.
	test, e := os.CreateTemp(filepath.Dir(exe), ".yago-write-")
	if e != nil {
		return errors.New("Нет прав на замену YaGo. Перенесите программу в свою папку")
	}
	test.Close()
	os.Remove(test.Name())
	dir, e := os.MkdirTemp(filepath.Dir(path), "installer-")
	if e != nil {
		return e
	}
	helper := filepath.Join(dir, "apply.exe")
	if e = copyFile(exe, helper); e != nil {
		return e
	}
	job := installJob{exe, path, filepath.Join(filepath.Dir(path), "updates.json"), state, filepath.Join(dir, "started"), uint32(os.Getpid())}
	b, _ := json.Marshal(job)
	jobFile := filepath.Join(dir, "job.json")
	if e = os.WriteFile(jobFile, b, 0600); e != nil {
		return e
	}
	cmd := exec.Command(helper, "--yago-apply", jobFile)
	hidden(cmd)
	if e = cmd.Start(); e != nil {
		return e
	}
	for i := 0; i < 100; i++ {
		if _, e = os.Stat(filepath.Join(dir, "ready")); e == nil {
			return nil
		}
		if b, e := os.ReadFile(filepath.Join(dir, "error.txt")); e == nil {
			return errors.New(string(b))
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("Установщик не ответил; программа остаётся открытой")
}

// RunInstaller executes only in the copied helper before Gio starts.
func RunInstaller() bool {
	if len(os.Args) != 3 || os.Args[1] != "--yago-apply" {
		return false
	}
	jobFile := os.Args[2]
	dir := filepath.Dir(jobFile)
	fail := func(e error) { _ = os.WriteFile(filepath.Join(dir, "error.txt"), []byte(e.Error()), 0600) }
	var j installJob
	b, e := os.ReadFile(jobFile)
	if e != nil {
		fail(e)
		return true
	}
	if json.Unmarshal(b, &j) != nil {
		fail(errors.New("invalid installer job"))
		return true
	}
	raw, e := os.ReadFile(j.Manifest)
	if e != nil {
		fail(e)
		return true
	}
	m, e := Verify(raw)
	if e != nil {
		fail(e)
		return true
	}
	a, ok := m.Artifacts["windows_amd64"]
	if !ok || !Newer(m.Version, Version) {
		fail(errors.New("invalid update version"))
		return true
	}
	if e = VerifyFile(j.Staged, a); e != nil {
		fail(e)
		return true
	}
	parent, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, j.PID)
	if e != nil {
		fail(e)
		return true
	}
	var image [32768]uint16
	n := uint32(len(image))
	e = windows.QueryFullProcessImageName(parent, 0, &image[0], &n)
	if e != nil || !samePath(windows.UTF16ToString(image[:n]), j.Target) {
		windows.CloseHandle(parent)
		fail(errors.New("parent executable mismatch"))
		return true
	}
	if e = os.WriteFile(filepath.Join(dir, "ready"), []byte("1"), 0600); e != nil {
		windows.CloseHandle(parent)
		fail(e)
		return true
	}
	result, e := windows.WaitForSingleObject(parent, 60000)
	windows.CloseHandle(parent)
	if e != nil || result != windows.WAIT_OBJECT_0 {
		fail(errors.New("application did not exit"))
		return true
	}
	if e = VerifyFile(j.Staged, a); e != nil {
		fail(e)
		return true
	}
	incoming := j.Target + ".incoming-" + strconv.Itoa(os.Getpid())
	backup := j.Target + ".previous-" + strconv.Itoa(os.Getpid())
	if e = copyFile(j.Staged, incoming); e != nil {
		fail(e)
		return true
	}
	defer os.Remove(incoming)
	if e = VerifyFile(incoming, a); e != nil {
		fail(e)
		return true
	}
	if e = os.Rename(j.Target, backup); e != nil {
		fail(e)
		return true
	}
	if e = os.Rename(incoming, j.Target); e != nil {
		_ = os.Rename(backup, j.Target)
		fail(e)
		return true
	}
	cmd := exec.Command(j.Target, "--state", j.State, "--update-ticket", j.Marker)
	e = cmd.Start()
	healthy := false
	if e == nil {
		for i := 0; i < 150; i++ {
			if b, _ := os.ReadFile(j.Marker); string(b) == m.Version {
				healthy = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	if healthy {
		os.Remove(backup)
		return true
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	failed := j.Target + ".failed-" + strconv.Itoa(os.Getpid())
	if e = os.Rename(j.Target, failed); e == nil {
		e = os.Rename(backup, j.Target)
		if e == nil {
			os.Remove(failed)
			retry := exec.Command(j.Target, "--state", j.State)
			_ = retry.Start()
		}
	}
	fail(fmt.Errorf("update startup failed; rollback: %v", e))
	return true
}
func samePath(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return windowsPathEqual(a, b)
}
