package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/hajimehoshi/go-mp3"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
	"yago/internal/audio"
	"yago/internal/metrics"
	"yago/internal/music"
)

func TestMemoryAuditProfile(t *testing.T) {
	dir := os.Getenv("YAGO_MEMORY_AUDIT_DIR")
	if dir == "" {
		t.Skip("opt-in synthetic memory audit")
	}
	scenario := os.Getenv("YAGO_MEMORY_AUDIT_SCENARIO")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	runtime.MemProfileRate = 4096
	capture := func(name string) {
		runtime.GC()
		sample := metrics.Current()
		b, _ := json.MarshalIndent(map[string]any{"sample": sample, "goroutines": runtime.NumGoroutine()}, "", "  ")
		if e := os.WriteFile(filepath.Join(dir, scenario+"-"+name+".json"), b, 0600); e != nil {
			t.Fatal(e)
		}
		f, e := os.Create(filepath.Join(dir, scenario+"-"+name+".pprof"))
		if e != nil {
			t.Fatal(e)
		}
		if e = pprof.WriteHeapProfile(f); e != nil {
			t.Fatal(e)
		}
		f.Close()
		t.Logf("%s %s: heap=%d GoSys=%d WS=%d private=%d goroutines=%d", scenario, name, sample.GoHeap, sample.GoSystem, sample.WorkingSet, sample.PrivateBytes, runtime.NumGoroutine())
	}
	switch scenario {
	case "clock":
		u := &UI{theme: material.NewTheme()}
		u.theme.Shaper = text.NewShaper(text.WithCollection(uiFonts()))
		var ops op.Ops
		render := func(position int) {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(800, 100)}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Now()}
			u.playbackClock(gtx, float64(position), 7200, 13)
		}
		render(0)
		capture("start")
		for i := 0; i < 10000; i++ {
			render(i % 7200)
		}
		capture("end")
		runtime.KeepAlive(u)
		runtime.KeepAlive(&ops)
	case "updates":
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		u := &UI{ctx: ctx}
		capture("start")
		for i := 0; i < 30000; i++ {
			_, id := u.updateJob("synthetic")
			u.finishUpdate(id, func() {})
		}
		capture("end")
		runtime.KeepAlive(ctx)
		runtime.KeepAlive(u)
	case "audio":
		file := os.Getenv("YAGO_TEST_MP3")
		if file == "" {
			t.Fatal("YAGO_TEST_MP3 required")
		}
		duration := func() float64 {
			f, e := os.Open(file)
			if e != nil {
				t.Fatal(e)
			}
			defer f.Close()
			d, e := mp3.NewDecoder(f)
			if e != nil {
				t.Fatal(e)
			}
			return float64(d.Length()) / float64(d.SampleRate()*4)
		}()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, file) }))
		defer server.Close()
		p := audio.New(t.TempDir(), func() {})
		defer p.Close()
		p.Volume(0)
		wait := func(ended bool) {
			deadline := time.Now().Add(20 * time.Second)
			for time.Now().Before(deadline) {
				s := p.Snapshot()
				if s.Error != "" {
					t.Fatal(s.Error)
				}
				if (ended && s.Ended) || (!ended && s.ID == 0) {
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatal("audio audit timed out", p.Snapshot())
		}
		resolve := func(context.Context) (string, error) { return server.URL, nil }
		p.PlayAt(resolve, max(0, duration-.2), false)
		wait(true)
		p.Stop()
		wait(false)
		capture("start")
		p.PlayAt(resolve, max(0, duration-.2), false)
		wait(true)
		capture("end")
		runtime.KeepAlive(p)
	default:
		t.Fatal(fmt.Sprintf("unknown audit scenario %q", scenario))
	}
}
func BenchmarkSingleArtistLabel(b *testing.B) {
	track := memoryAuditTrack()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		memoryAuditArtist = track.Artist()
	}
}

var memoryAuditArtist string

func memoryAuditTrack() music.Track {
	return music.Track{Artists: []music.Named{{Name: "Synthetic artist"}}}
}
