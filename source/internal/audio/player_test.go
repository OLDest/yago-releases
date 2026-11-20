package audio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/go-mp3"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("YAGO_TEST_MP3")
	if path == "" {
		t.Skip("set YAGO_TEST_MP3 to run real MP3 integration checks")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if len(b) < 65536 {
		t.Fatal("fixture too short")
	}
	return b
}
func TestRealMP3DecodesBeforeDownloadFinishes(t *testing.T) {
	b := fixture(t)
	finish := make(chan struct{})
	defer close(finish)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(b[:65536])
		w.(http.Flusher).Flush()
		select {
		case <-finish:
			w.Write(b[65536:])
		case <-r.Context().Done():
		}
	}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sp, e := OpenStream(ctx, s.URL, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer sp.Close()
	d, e := mp3.NewDecoder(sp.Reader(ctx))
	if e != nil {
		t.Fatal(e)
	}
	pcm := make([]byte, 4096)
	if _, e = io.ReadFull(d, pcm); e != nil {
		t.Fatal(e)
	}
	if _, done, _ := sp.Status(); done {
		t.Fatal("decoder waited for full file")
	}
}
func TestRealAudioPauseSeekAndCancel(t *testing.T) {
	b := fixture(t)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer s.Close()
	p := New(t.TempDir(), nil)
	defer p.Close()
	p.Volume(0)
	resolve := func(context.Context) (string, error) { return s.URL, nil }
	p.Play(resolve)
	wait := func(check func(Snapshot) bool) {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			v := p.Snapshot()
			if v.Error != "" {
				t.Fatal(v.Error)
			}
			if check(v) {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatal("playback timed out", p.Snapshot())
	}
	wait(func(v Snapshot) bool { return v.Playing && v.Position > .1 && v.Downloaded == int64(len(b)) })
	p.Toggle()
	wait(func(v Snapshot) bool { return !v.Playing })
	before := p.Snapshot().Position
	time.Sleep(400 * time.Millisecond)
	if p.Snapshot().Position > before+.1 {
		t.Fatal("pause advances position")
	}
	p.Seek(1)
	wait(func(v Snapshot) bool { return !v.Loading && v.Position >= .9 })
	p.Toggle()
	wait(func(v Snapshot) bool { return v.Playing })
	p.Seek(1e6)
	wait(func(v Snapshot) bool { return v.Ended })
	for i := 0; i < 10; i++ {
		p.Play(resolve)
		p.Stop()
	}
	wait(func(v Snapshot) bool { return !v.Playing && !v.Loading })
}
