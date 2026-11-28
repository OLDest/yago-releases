package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"testing"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/unit"
	"yago/internal/metrics"
	"yago/internal/store"
)

// Opt-in real MP3 playback through the complete window/controller/audio path.
// Samples never force GC or trim the process working set during playback.
func TestNativePlaybackMemory(t *testing.T) {
	dir, fixture := os.Getenv("YAGO_PLAYBACK_MEMORY_DIR"), os.Getenv("YAGO_TEST_MP3")
	if dir == "" || fixture == "" {
		t.Skip("opt-in live playback audit")
	}
	fixture, _ = filepath.Abs(fixture)
	if _, err := os.Stat(fixture); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, fixture) }))
	defer server.Close()
	stateDir = filepath.Join(dir, "state")
	demo, demoURL = true, server.URL
	defer func() { demo = false; demoURL = "" }()
	debug.SetMemoryLimit(96 << 20)
	debug.SetGCPercent(75)
	runtime.MemProfileRate = 4096
	tracks := demoTracks()
	if err := store.SavePlayback(stateDir, store.Playback{Version: 1, Owner: "__demo__", Demo: true, Queue: tracks, Order: []int{0, 1, 2}, Index: 0, Playing: true, Volume: .25}); err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(filepath.Join(dir, "samples.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	enc := json.NewEncoder(output)
	start := time.Now()
	capture := func(phase string) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		p, e := store.LoadPlayback(stateDir)
		entry := map[string]any{"seconds": time.Since(start).Seconds(), "phase": phase, "memory": metrics.Current(), "goroutines": runtime.NumGoroutine(), "heap_inuse": m.HeapInuse, "heap_idle": m.HeapIdle, "heap_released": m.HeapReleased, "heap_objects": m.HeapObjects, "mallocs": m.Mallocs, "frees": m.Frees, "num_gc": m.NumGC}
		if e == nil {
			entry["position"] = p.Position
			entry["index"] = p.Index
			entry["playing"] = p.Playing
		}
		if e := enc.Encode(entry); e != nil {
			t.Error(e)
		}
		output.Sync()
		if phase == "playing" && time.Since(start) < 12*time.Second {
			if f, e := os.Create(filepath.Join(dir, "goroutines.txt")); e == nil {
				pprof.Lookup("goroutine").WriteTo(f, 2)
				f.Close()
			}
		}
	}
	capture("before-window")
	w := new(app.Window)
	w.Option(app.Title("YaGo playback memory audit"), app.Size(unit.Dp(1080), unit.Dp(740)))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 72; i++ {
			time.Sleep(5 * time.Second)
			capture("playing")
			if p, e := store.LoadPlayback(stateDir); e == nil && !p.Playing {
				t.Error("playback stopped before audit completed; inspect audio device")
				break
			}
		}
		w.Perform(system.ActionClose)
	}()
	if err := run(w); err != nil {
		t.Error(err)
	}
	<-done
	capture("closed")
	runtime.GC()
	capture("closed-after-gc")
	f, err := os.Create(filepath.Join(dir, "closed.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	pprof.WriteHeapProfile(f)
	p, err := store.LoadPlayback(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Position < 10 && p.Index == 0 {
		t.Fatalf("audio did not advance: %+v", p)
	}
}
