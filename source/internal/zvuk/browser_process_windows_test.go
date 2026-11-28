package zvuk

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Simulate Chromium's launcher handing off to a different process. The initial
// process exits successfully while the owned browser job must stay alive.
func TestBrowserJobSurvivesLauncherHandoff(t *testing.T) {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		t.Fatal(e)
	}
	t.Setenv("YAGO_JOB_TEST_ROLE", "launcher")
	t.Setenv("YAGO_JOB_TEST_MARKER", filepath.Join(t.TempDir(), "child-ready"))
	process, e := startBrowserProcess(os.Args[0], []string{"-test.run=^TestBrowserHandoffChild$"}, job)
	if e != nil {
		t.Fatal(e)
	}
	defer stopBrowserJob(job, process)
	event, e := windows.WaitForSingleObject(process, 10000)
	if e != nil || event != windows.WAIT_OBJECT_0 {
		t.Fatal("launcher did not finish", event, e)
	}
	if active, e := browserJobActive(job); e != nil || !active {
		t.Fatal("live browser lost when launcher exited", e)
	}
	if e = windows.TerminateJobObject(job, 0); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		active, e := browserJobActive(job)
		if e != nil {
			t.Fatal(e)
		}
		if !active {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("browser descendants survived cancellation")
}
func TestBrowserHandoffChild(t *testing.T) {
	role := os.Getenv("YAGO_JOB_TEST_ROLE")
	if role == "" {
		t.Skip("process fixture")
	}
	marker := os.Getenv("YAGO_JOB_TEST_MARKER")
	if role == "worker" {
		if e := os.WriteFile(marker, []byte("ready"), 0600); e != nil {
			os.Exit(2)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestBrowserHandoffChild$")
	child.Env = append(os.Environ(), "YAGO_JOB_TEST_ROLE=worker")
	child.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	if e := child.Start(); e != nil {
		os.Exit(3)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, e := os.Stat(marker); e == nil {
			os.Exit(0)
		}
		time.Sleep(20 * time.Millisecond)
	}
	os.Exit(4)
}
