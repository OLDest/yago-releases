package main

import (
	"encoding/json"
	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
	"yago/internal/metrics"
)

func TestNativeMemoryAudit(t *testing.T) {
	dir := os.Getenv("YAGO_NATIVE_MEMORY_DIR")
	if dir == "" {
		t.Skip("opt-in native GUI audit")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	stateDir = t.TempDir()
	demo = true
	defer func() { demo = false }()
	runtime.MemProfileRate = 4096
	capture := func(name string) {
		raw := metrics.Current()
		imageInfo := nativeImageBSS()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		value := map[string]any{"image_bss": imageInfo, "raw": raw, "after_gc": metrics.Current(), "goroutines": runtime.NumGoroutine(), "heap_idle": m.HeapIdle, "heap_released": m.HeapReleased}
		b, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			t.Error(e)
			return
		}
		if e = os.WriteFile(filepath.Join(dir, name+".json"), b, 0600); e != nil {
			t.Error(e)
		}
		f, e := os.Create(filepath.Join(dir, name+".pprof"))
		if e != nil {
			t.Error(e)
			return
		}
		pprof.WriteHeapProfile(f)
		f.Close()
		t.Logf("%s: %s", name, b)
	}
	capture("core")
	w := new(app.Window)
	w.Option(app.Title("YaGo native memory audit"), app.Size(unit.Dp(1080), unit.Dp(740)))
	if os.Getenv("YAGO_NATIVE_MEMORY_MODE") == "window-only" {
		w.Option(app.CustomRenderer(true))
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, name := range []string{"ui-10s", "ui-20s", "ui-30s"} {
			time.Sleep(10 * time.Second)
			capture(name)
		}
		w.Perform(system.ActionClose)
	}()
	var runErr error
	if mode := os.Getenv("YAGO_NATIVE_MEMORY_MODE"); mode == "blank" || mode == "window-only" {
		runErr = runNativeBlank(w)
	} else {
		runErr = run(w)
	}
	if e := runErr; e != nil {
		t.Error(e)
	}
	<-done
	capture("closed")
}

func runNativeBlank(w *app.Window) error {
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			paint.Fill(gtx.Ops, color.NRGBA{R: 17, G: 19, B: 24, A: 255})
			e.Frame(gtx.Ops)
		}
	}
}
