package winkeys

import (
	"context"
	"golang.org/x/sys/windows"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestNativeHotkeyMessageLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	received := make(chan string, 32)
	var status Status
	select {
	case status = <-Start(ctx, func(command string) { received <- command }):
	case <-time.After(5 * time.Second):
		t.Fatal("no native thread")
	}
	for i, b := range Bindings() {
		r, _, e := postMessage.Call(uintptr(status.thread), 0x312, uintptr(i+1), 0)
		if r == 0 {
			t.Fatal(e)
		}
		select {
		case got := <-received:
			if got != b.Command {
				t.Fatalf("%s != %s", got, b.Command)
			}
		case <-time.After(time.Second):
			t.Fatal("command not delivered")
		}
	}
	cancel()
	select {
	case <-status.done:
	case <-time.After(time.Second):
		t.Fatal("native thread did not exit")
	}
	t.Logf("native messages delivered; unavailable registrations: %v", status.Unavailable)
}

// Tests only a private, hidden native window; no user windows are messaged.
func TestNativeAppCommandSubclass(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, _ := windows.UTF16PtrFromString("STATIC")
	create := user.NewProc("CreateWindowExW")
	destroy := user.NewProc("DestroyWindow")
	send := user.NewProc("SendMessageW")
	hwnd, _, e := create.Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0)
	if hwnd == 0 {
		t.Fatal(e)
	}
	defer destroy.Call(hwnd)
	got := ""
	if !Attach(hwnd, func(command string) { got = command }) {
		t.Fatal("subclass failed")
	}
	for id, want := range map[uintptr]string{8: "mute", 9: "volume-down", 10: "volume-up", 11: "next", 12: "previous", 13: "stop", 14: "toggle", 46: "play", 47: "pause"} {
		got = ""
		r, _, _ := send.Call(hwnd, 0x319, 0, (id|0x8000)<<16)
		if r != 1 || got != want {
			t.Fatalf("command %d: result %d, got %s", id, r, got)
		}
	}
}
