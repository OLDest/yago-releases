package audio

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStalledBodyIsCancelledAndDoesNotRetainDownloader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ID3"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	f, err := os.Create(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s := &Spool{file: f, cancel: cancel}
	s.cond = sync.NewCond(&s.mu)
	if err = s.downloadWithIdleTimeout(ctx, server.URL, 100*time.Millisecond); !errors.Is(err, errAudioIdle) {
		t.Fatal("stalled body did not report inactivity", err)
	}
	if ctx.Err() != nil {
		t.Fatal("parent track context was cancelled")
	}
}
