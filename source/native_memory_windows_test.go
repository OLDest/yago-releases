//go:build windows

package main

import (
	"debug/pe"
	"golang.org/x/sys/windows"
	"os"
	"runtime/pprof"
	"testing"
	"unsafe"
)

func nativeImageBSS() any {
	executable, e := os.Executable()
	if e != nil {
		return e.Error()
	}
	file, e := pe.Open(executable)
	if e != nil {
		return e.Error()
	}
	defer file.Close()
	var module windows.Handle
	if e = windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &module); e != nil {
		return e.Error()
	}
	var pages []struct{ address, attributes uintptr }
	var reserved uint64
	for _, section := range file.Sections {
		if section.VirtualSize <= section.Size {
			continue
		}
		start := uintptr(module) + uintptr(section.VirtualAddress) + uintptr(section.Size)
		end := uintptr(module) + uintptr(section.VirtualAddress) + uintptr(section.VirtualSize)
		reserved += uint64(end - start)
		for at := (start + 4095) &^ uintptr(4095); at+4096 <= end; at += 4096 {
			pages = append(pages, struct{ address, attributes uintptr }{address: at})
		}
	}
	if len(pages) == 0 {
		return nil
	}
	if e = windows.QueryWorkingSetEx(windows.CurrentProcess(), uintptr(unsafe.Pointer(&pages[0])), uint32(len(pages))*uint32(unsafe.Sizeof(pages[0]))); e != nil {
		return e.Error()
	}
	resident, private := 0, 0
	for _, page := range pages {
		if page.attributes&1 != 0 {
			resident++
			if page.attributes&(1<<15) == 0 {
				private++
			}
		}
	}
	return map[string]any{"virtual_zero_fill_bytes": reserved, "resident_full_pages_bytes": resident * 4096, "private_resident_full_pages_bytes": private * 4096}
}

func TestProfilerResidencyAudit(t *testing.T) {
	if os.Getenv("YAGO_NATIVE_MEMORY_DIR") == "" {
		t.Skip("opt-in")
	}
	t.Log("before profile", nativeImageBSS())
	f, e := os.CreateTemp(t.TempDir(), "heap")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if e = pprof.WriteHeapProfile(f); e != nil {
		t.Fatal(e)
	}
	t.Log("after profile", nativeImageBSS())
}
