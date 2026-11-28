package winkeys

import (
	"context"
	"golang.org/x/sys/windows"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var user = windows.NewLazySystemDLL("user32.dll")
var register = user.NewProc("RegisterHotKey")
var unregister = user.NewProc("UnregisterHotKey")
var getMessage = user.NewProc("GetMessageW")
var peekMessage = user.NewProc("PeekMessageW")
var postMessage = user.NewProc("PostThreadMessageW")
var common = windows.NewLazySystemDLL("comctl32.dll")
var setSubclass = common.NewProc("SetWindowSubclass")
var defSubclass = common.NewProc("DefSubclassProc")
var targets sync.Map
var callback = syscall.NewCallback(func(hwnd, msg, wp, lp, id, ref uintptr) uintptr {
	if v, ok := targets.Load(hwnd); ok {
		if msg == 0x319 {
			if command := AppCommand(lp); command != "" {
				v.(func(string))(command)
				return 1
			}
		}
		if msg == 0x82 {
			targets.Delete(hwnd)
		}
	}
	r, _, _ := defSubclass.Call(hwnd, msg, wp, lp)
	return r
})

// Attach must run on the native window thread (Gio Window.Run).
func Attach(hwnd uintptr, emit func(string)) bool {
	targets.Store(hwnd, emit)
	r, _, _ := setSubclass.Call(hwnd, callback, 0x5941474F, 0)
	if r == 0 {
		targets.Delete(hwnd)
	}
	return r != 0
}

type message struct {
	HWND           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}
type Status struct {
	Unavailable []string
	thread      uint32
	done        <-chan struct{}
}

func Start(ctx context.Context, emit func(string)) <-chan Status {
	ready := make(chan Status, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		done := make(chan struct{})
		defer close(done)
		var m message
		peekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 0)
		thread := windows.GetCurrentThreadId()
		bindings := Bindings()
		bound := []int{}
		missing := []string{}
		for i, b := range bindings {
			flags := b.Modifiers
			if b.Command != "volume-up" && b.Command != "volume-down" {
				flags |= 0x4000
			}
			r, _, _ := register.Call(0, uintptr(i+1), flags, b.Key)
			if r != 0 {
				bound = append(bound, i+1)
			} else {
				missing = append(missing, b.Name)
			}
		}
		defer func() {
			for _, id := range bound {
				unregister.Call(0, uintptr(id))
			}
		}()
		ready <- Status{missing, thread, done}
		go func() {
			select {
			case <-ctx.Done():
				postMessage.Call(uintptr(thread), 0x12, 0, 0)
			case <-done:
			}
		}()
		for {
			r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			if m.Message == 0x312 {
				if id := int(m.WParam); id > 0 && id <= len(bindings) {
					emit(bindings[id-1].Command)
				}
			}
		}
	}()
	return ready
}
