package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestResamplerOddReadsDoNotAllocatePerSample(t *testing.T) {
	data := make([]byte, 4800*4)
	for i := 0; i < len(data); i += 4 {
		binary.LittleEndian.PutUint16(data[i:], 1200)
		binary.LittleEndian.PutUint16(data[i+2:], uint16(64336))
	}
	all, e := io.ReadAll(newResampler(bytes.NewReader(data), 48000))
	if e != nil {
		t.Fatal(e)
	}
	r := newResampler(bytes.NewReader(data), 48000)
	var got bytes.Buffer
	dst := make([]byte, 7)
	for {
		n, e := r.Read(dst)
		got.Write(dst[:n])
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	if !bytes.Equal(got.Bytes(), all) {
		t.Fatal("partial PCM frame changed")
	}
	allocs := testing.AllocsPerRun(5, func() {
		r := newResampler(bytes.NewReader(data), 48000)
		for {
			_, e := r.Read(dst)
			if e == io.EOF {
				break
			}
			if e != nil {
				panic(e)
			}
		}
	})
	if allocs > 8 {
		t.Fatalf("per-sample allocations: %.0f", allocs)
	}
}
func TestCachedSpoolReadsDoNotAllocateCallbacks(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "ready")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	f.Write(make([]byte, 4096))
	s := &Spool{file: f, size: 4096, done: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dst := make([]byte, 4)
	r := &streamReader{s: s, ctx: ctx}
	allocs := testing.AllocsPerRun(10, func() {
		r.offset = 0
		for i := 0; i < 1024; i++ {
			if _, e := r.Read(dst); e != nil {
				panic(e)
			}
		}
	})
	// ReadAt yields to Windows. Global allocation counters can include a few
	// allocations from the audio/network workers of earlier integration tests.
	// A per-read cancellation callback causes thousands here, not fixed noise.
	if allocs > 8 {
		t.Fatalf("cached reads allocate: %.0f", allocs)
	}
}
func TestRapidTrackChangesReleaseResources(t *testing.T) {
	file := os.Getenv("YAGO_TEST_MP3")
	if file == "" {
		t.Skip("YAGO_TEST_MP3 required")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, file) }))
	defer server.Close()
	dir := t.TempDir()
	p := New(dir, nil)
	defer p.Close()
	p.Volume(0)
	resolve := func(context.Context) (string, error) { return server.URL, nil }
	wait := func(id uint64) {
		t.Helper()
		until := time.Now().Add(10 * time.Second)
		for time.Now().Before(until) {
			s := p.Snapshot()
			if s.Error != "" {
				t.Fatal(s.Error)
			}
			if s.ID == id && !s.Loading {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("actor timed out", p.Snapshot())
	}
	p.PlayAt(resolve, 0, true)
	wait(p.sequence.Load())
	p.Stop()
	wait(0)
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	goroutines := runtime.NumGoroutine()
	for i := 0; i < 30; i++ {
		id := p.PlayAt(resolve, 0, true)
		wait(id)
		p.Seek(.05)
		p.Stop()
		wait(0)
	}
	until := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > goroutines+2 && time.Now().Before(until) {
		time.Sleep(20 * time.Millisecond)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 0 {
		t.Fatal("spool remains after stop", len(entries), e)
	}
	if runtime.NumGoroutine() > goroutines+2 {
		t.Fatal("worker goroutines retained", goroutines, runtime.NumGoroutine())
	}
	t.Logf("30 switches: heap %d -> %d; goroutines %d -> %d", before.HeapAlloc, after.HeapAlloc, goroutines, runtime.NumGoroutine())
	if after.HeapAlloc > before.HeapAlloc+2<<20 {
		t.Fatal("retained heap grew over 2 MiB")
	}
}
