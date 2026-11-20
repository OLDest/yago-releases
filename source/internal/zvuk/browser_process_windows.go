package zvuk

import (
	"fmt"
	"golang.org/x/sys/windows"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Windows 10 assigns the job before any child code can run. Assigning after
// exec.Cmd.Start allows a Chromium launcher to hand off before it is contained.
// https://devblogs.microsoft.com/oldnewthing/20230209-00/?p=107812
func startBrowserProcess(exe string, args []string, job windows.Handle) (windows.Handle, error) {
	attrs, e := windows.NewProcThreadAttributeList(1)
	if e != nil {
		return 0, e
	}
	defer attrs.Delete()
	const jobListAttribute = 0x0002000d // PROC_THREAD_ATTRIBUTE_JOB_LIST
	if e = attrs.Update(jobListAttribute, unsafe.Pointer(&job), unsafe.Sizeof(job)); e != nil {
		return 0, e
	}
	si := windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESHOWWINDOW
	si.ShowWindow = windows.SW_SHOWNORMAL
	si.ProcThreadAttributeList = attrs.List()
	parts := []string{syscall.EscapeArg(exe)}
	for _, arg := range args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	app, e := windows.UTF16PtrFromString(exe)
	if e != nil {
		return 0, e
	}
	command, e := windows.UTF16PtrFromString(strings.Join(parts, " "))
	if e != nil {
		return 0, e
	}
	var process windows.ProcessInformation
	if e = windows.CreateProcess(app, command, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT, nil, nil, &si.StartupInfo, &process); e != nil {
		return 0, e
	}
	windows.CloseHandle(process.Thread)
	return process.Process, nil
}

type browserJobAccounting struct {
	TotalUserTime, TotalKernelTime, ThisPeriodUserTime, ThisPeriodKernelTime       int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses uint32
}

func browserJobActive(job windows.Handle) (bool, error) {
	var info browserJobAccounting
	e := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
	return info.ActiveProcesses > 0, e
}
func browserStartupExit(process windows.Handle) error {
	var code uint32
	if windows.GetExitCodeProcess(process, &code) != nil {
		return fmt.Errorf("Браузер завершился при запуске")
	}
	return fmt.Errorf("Браузер завершился при запуске (код %d)", code)
}
func stopBrowserJob(job, process windows.Handle) {
	_ = windows.TerminateJobObject(job, 0)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		active, e := browserJobActive(job)
		if e != nil || !active {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	windows.CloseHandle(process)
}
