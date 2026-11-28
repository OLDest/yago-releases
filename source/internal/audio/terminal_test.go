package audio

import (
	"context"
	"github.com/hajimehoshi/go-mp3"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func terminalWait(t *testing.T, p *Player, check func(Snapshot) bool) {
	t.Helper()
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		if check(p.Snapshot()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("terminal audio timeout", p.Snapshot())
}
func TestSeekStillWorksAfterEnd(t *testing.T) {
	path := os.Getenv("YAGO_TEST_MP3")
	if path == "" {
		t.Skip("real MP3 fixture required")
	}
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	d, e := mp3.NewDecoder(f)
	if e != nil {
		t.Fatal(e)
	}
	duration := float64(d.Length()) / float64(d.SampleRate()*4)
	f.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, path) }))
	defer server.Close()
	p := New(t.TempDir(), func() {})
	defer p.Close()
	p.Volume(0)
	p.PlayAt(func(context.Context) (string, error) { return server.URL, nil }, max(0, duration-.2), false)
	terminalWait(t, p, func(s Snapshot) bool { return s.Ended || s.Error != "" })
	if s := p.Snapshot(); s.Error != "" {
		t.Fatal(s.Error)
	}
	p.Seek(0)
	terminalWait(t, p, func(s Snapshot) bool { return !s.Loading && !s.Ended && s.Playing })
}
func TestFailedAudioReleasesTemporaryFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "failed", 500) }))
	defer server.Close()
	dir := t.TempDir()
	p := New(dir, func() {})
	defer p.Close()
	id := p.Play(func(context.Context) (string, error) { return server.URL, nil })
	terminalWait(t, p, func(s Snapshot) bool { return s.Error != "" })
	if s := p.Snapshot(); s.ID != id || s.DeviceError || s.Loading {
		t.Fatal("HTTP failure lost track identity or was classified as device failure", s)
	}
	files, e := filepath.Glob(filepath.Join(dir, "audio-*.part"))
	if e != nil || len(files) != 0 {
		t.Fatal("failed stream retained disk file", files, e)
	}
}
