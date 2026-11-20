package audio

import (
	"context"
	"encoding/json"
	"github.com/hajimehoshi/go-mp3"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"
	"time"
	"yago/internal/metrics"
)

// Exercises real network spool, MP3 decoder and resampler at listening speed.
// This deliberately does not stand in for a working OS audio output device.
func TestPacedDecodeMemory(t *testing.T) {
	dir, fixture := os.Getenv("YAGO_DECODE_MEMORY_DIR"), os.Getenv("YAGO_TEST_MP3")
	if dir == "" || fixture == "" {
		t.Skip("opt-in paced MP3 memory audit")
	}
	fixture, _ = filepath.Abs(fixture)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, fixture) }))
	defer server.Close()
	out, err := os.Create(filepath.Join(dir, "samples.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	debug.SetMemoryLimit(96 << 20)
	debug.SetGCPercent(75)
	enc := json.NewEncoder(out)
	start := time.Now()
	var decoded int64
	capture := func(phase string) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		enc.Encode(map[string]any{"seconds": time.Since(start).Seconds(), "phase": phase, "memory": metrics.Current(), "goroutines": runtime.NumGoroutine(), "decoded_bytes": decoded, "heap_objects": m.HeapObjects, "num_gc": m.NumGC})
		out.Sync()
	}
	runtime.GC()
	capture("before")
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	buf := make([]byte, Rate*4/50)
	cache := filepath.Join(dir, "cache")
	for track := 0; track < 3; track++ {
		ctx, cancel := context.WithCancel(context.Background())
		s, err := OpenStream(ctx, server.URL, cache)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		source, err := newMP3Source(s.Reader(ctx))
		if err != nil {
			s.Close()
			cancel()
			t.Fatal(err)
		}
		dec, err := mp3.NewDecoder(source)
		if err != nil {
			s.Close()
			cancel()
			t.Fatal(err)
		}
		pcm := newResampler(dec, dec.SampleRate())
		for frame := 0; frame < 1500; frame++ {
			n, err := io.ReadFull(pcm, buf)
			decoded += int64(n)
			if err != nil {
				s.Close()
				cancel()
				t.Fatal(err)
			}
			time.Sleep(20 * time.Millisecond)
			if frame%250 == 249 {
				capture("decoding")
			}
		}
		s.Close()
		cancel()
		capture("track-closed")
	}
	runtime.GC()
	capture("after-gc")
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	t.Logf("decoded %d PCM bytes; retained heap %d -> %d", decoded, before.HeapAlloc, after.HeapAlloc)
	if after.HeapAlloc > before.HeapAlloc+2<<20 {
		t.Fatal("more than 2 MiB retained after three tracks")
	}
	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) != 0 {
		t.Fatal("spool files remain", err, len(entries))
	}
}
