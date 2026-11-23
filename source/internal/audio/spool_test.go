package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestProgressivePlaybackAndCleanup(t *testing.T) {
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("token sent to CDN")
		}
		w.Write([]byte("first"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Write([]byte("second"))
	}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sp, e := OpenStream(ctx, s.URL, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	name := sp.file.Name()
	r := sp.Reader(ctx)
	b := make([]byte, 5)
	if _, e = io.ReadFull(r, b); e != nil || string(b) != "first" {
		t.Fatal(string(b), e)
	}
	if _, done, _ := sp.Status(); done {
		t.Fatal("waited for full download")
	}
	close(release)
	tail, e := io.ReadAll(r)
	if e != nil || string(tail) != "second" {
		t.Fatal(string(tail), e)
	}
	replay, e := io.ReadAll(sp.Reader(ctx))
	if e != nil || string(replay) != "firstsecond" {
		t.Fatal(string(replay), e)
	}
	sp.Close()
	if _, e = os.Stat(name); !os.IsNotExist(e) {
		t.Fatal("temporary audio remains", e)
	}
}
func TestCancellationUnblocksReader(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer s.Close()
	root, cancel := context.WithCancel(context.Background())
	sp, e := OpenStream(root, s.URL, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	readCtx, stop := context.WithCancel(root)
	done := make(chan error, 1)
	go func() { _, e := sp.Reader(readCtx).Read(make([]byte, 1)); done <- e }()
	stop()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("expected cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked reader")
	}
	cancel()
	sp.Close()
}
func TestRejectOversizedTrack(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999999999")
		w.WriteHeader(200)
	}))
	defer s.Close()
	sp, e := OpenStream(context.Background(), s.URL, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer sp.Close()
	if _, e = io.ReadAll(sp.Reader(context.Background())); e == nil {
		t.Fatal("size limit missing")
	}
}
func TestResamplingBoundedAndStereo(t *testing.T) {
	var raw bytes.Buffer
	for i := 0; i < 480; i++ {
		binary.Write(&raw, binary.LittleEndian, int16(1200))
		binary.Write(&raw, binary.LittleEndian, int16(-1200))
	}
	r := newResampler(&raw, 48000)
	out, e := io.ReadAll(r)
	if e != nil {
		t.Fatal(e)
	}
	if len(out) < 440*4 || len(out) > 442*4 {
		t.Fatal("wrong output duration", len(out))
	}
	for i := 0; i < len(out); i += 4 {
		if int16(binary.LittleEndian.Uint16(out[i:])) != 1200 || int16(binary.LittleEndian.Uint16(out[i+2:])) != -1200 {
			t.Fatal("stereo channels changed")
		}
	}
}
