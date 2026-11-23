package offline

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"yago/internal/music"
)

func mp3Fixture(t *testing.T) []byte {
	t.Helper()
	name := os.Getenv("YAGO_TEST_MP3")
	if name == "" {
		t.Skip("YAGO_TEST_MP3 required")
	}
	f, e := os.Open(name)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 128<<10))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func waitManager(t *testing.T, m *Manager, n int) Progress {
	t.Helper()
	until := time.Now().Add(10 * time.Second)
	for time.Now().Before(until) {
		p := m.Snapshot()
		if !p.Active && p.Pending == 0 && p.Done+p.Failed >= n {
			return p
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("download timed out", m.Snapshot())
	return Progress{}
}
func TestDownloadDefaultDedupReuseAndNoCredentials(t *testing.T) {
	b := mp3Fixture(t)
	var requests atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("API credentials sent to CDN")
		}
		w.Write(b)
	}))
	defer s.Close()
	l, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	m := NewManager(context.Background(), l, nil)
	defer m.Close()
	j := Job{Track: music.Track{ID: "one", Title: "Song"}, Quality: music.Normal, Resolve: func(context.Context, music.Track, music.Quality) (music.StreamInfo, error) {
		return music.StreamInfo{URL: s.URL, Bitrate: 192}, nil
	}}
	if n, e := m.Enqueue([]Job{j, j}); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if p := waitManager(t, m, 1); p.Failed != 0 {
		t.Fatal(p)
	}
	path, saved, e := l.Resolve("one")
	if e != nil || saved.Size != int64(len(b)) || saved.Bitrate != 192 {
		t.Fatal(saved, e)
	}
	if len(l.Tracks("")) != 1 {
		t.Fatal("default playlist missing")
	}
	custom, e := l.Create("Дорога")
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	j.Playlist = custom
	j.Resolve = nil
	if _, e = m.Enqueue([]Job{j}); e != nil {
		t.Fatal(e)
	}
	if p := waitManager(t, m, 1); p.Failed != 0 {
		t.Fatal(p)
	}
	if requests.Load() != 1 || len(l.Tracks(custom)) != 1 {
		t.Fatal("file redownloaded", requests.Load())
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal(e)
	}
}
func TestCancelDropsQueueAndPartialFile(t *testing.T) {
	b := mp3Fixture(t)
	var requests atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Write(b[:32768])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer s.Close()
	dir := t.TempDir()
	l, _ := Open(dir)
	m := NewManager(context.Background(), l, nil)
	defer m.Close()
	resolve := func(context.Context, music.Track, music.Quality) (music.StreamInfo, error) {
		return music.StreamInfo{URL: s.URL}, nil
	}
	m.Enqueue([]Job{{Track: music.Track{ID: "one"}, Resolve: resolve}, {Track: music.Track{ID: "two"}, Resolve: resolve}})
	until := time.Now().Add(5 * time.Second)
	for m.Snapshot().Downloaded == 0 && time.Now().Before(until) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.Snapshot().Downloaded == 0 {
		t.Fatal("download did not start")
	}
	m.Cancel()
	until = time.Now().Add(3 * time.Second)
	for m.Snapshot().Active && time.Now().Before(until) {
		time.Sleep(10 * time.Millisecond)
	}
	if p := m.Snapshot(); p.Active || p.Pending != 0 || p.Done != 0 {
		t.Fatal(p)
	}
	if requests.Load() != 1 || len(l.Tracks("")) != 0 {
		t.Fatal("canceled track published")
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "audio"))
	if len(entries) != 0 {
		t.Fatal("partial file remains", entries)
	}
}
func TestIncompleteOrInvalidAudioIsNotPublished(t *testing.T) {
	b := mp3Fixture(t)
	for _, kind := range []string{"truncated", "html", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "truncated":
					w.Header().Set("Content-Length", fmt.Sprint(len(b)+100))
					w.Write(b)
				case "html":
					w.Write([]byte("<html>error</html>"))
				case "oversized":
					w.Header().Set("Content-Length", "999999999")
					w.WriteHeader(200)
				}
			}))
			defer s.Close()
			l, _ := Open(t.TempDir())
			m := NewManager(context.Background(), l, nil)
			defer m.Close()
			m.Enqueue([]Job{{Track: music.Track{ID: "one"}, Resolve: func(context.Context, music.Track, music.Quality) (music.StreamInfo, error) {
				return music.StreamInfo{URL: s.URL}, nil
			}}})
			p := waitManager(t, m, 1)
			if p.Failed != 1 || len(l.Tracks("")) != 0 {
				t.Fatal(p)
			}
			entries, _ := os.ReadDir(filepath.Join(l.dir, "audio"))
			if len(entries) != 0 {
				t.Fatal("failed download retained")
			}
		})
	}
}
