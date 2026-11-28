package metrics

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func platform(s *Sample) {
	type counters struct {
		Size, Faults                                                                                                          uint32
		PeakWorkingSet, WorkingSet, PeakPagedPool, PagedPool, PeakNonPagedPool, NonPagedPool, Pagefile, PeakPagefile, Private uintptr
	}
	var c counters
	c.Size = uint32(unsafe.Sizeof(c))
	proc := windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	ok, _, _ := proc.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&c)), uintptr(c.Size))
	if ok != 0 {
		s.WorkingSet = uint64(c.WorkingSet)
		s.PrivateBytes = uint64(c.Private)
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user) == nil {
		s.CPUSeconds = float64(uint64(kernel.HighDateTime)<<32|uint64(kernel.LowDateTime))/1e7 + float64(uint64(user.HighDateTime)<<32|uint64(user.LowDateTime))/1e7
	}
}
