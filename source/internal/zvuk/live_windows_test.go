package zvuk

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/go-mp3"
	"yago/internal/music"
	"yago/internal/store"
)

// Read only YaGo's own explicitly selected credential. Never log token or signed CDN URL.
func TestLiveAuthenticatedMP3(t *testing.T) {
	state := os.Getenv("YAGO_ZVUK_LIVE_STATE")
	if state == "" {
		t.Skip("explicit local account check")
	}
	device, e := os.ReadFile(filepath.Join(state, "device-id"))
	if e != nil || len(device) != 32 {
		t.Fatal("YaGo device unavailable")
	}
	token, e := store.Load(string(device) + ":zvuk")
	if e != nil {
		t.Fatal("No saved Zvuk account")
	}
	c := New(token.Access)
	defer c.Close()
	c.AccountHint = token.Profile
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if _, e := c.Account(ctx); e != nil {
		t.Fatal("Saved account validation failed:", e)
	}
	var track music.Track
	if p, e := store.LoadPlayback(state); e == nil && music.IsZvuk(p.Queue[p.Index].ID) {
		track = p.Queue[p.Index]
	} else {
		tracks, e := c.Search(ctx, "пушной")
		if e != nil || len(tracks) == 0 {
			t.Fatal("Native search unavailable")
		}
		track = tracks[0]
	}
	info, e := c.Stream(ctx, track, music.High)
	if e != nil {
		t.Fatal(e)
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", info.URL, nil)
	h := &http.Client{Timeout: 20 * time.Second}
	defer h.CloseIdleConnections()
	r, e := h.Do(req)
	if e != nil {
		t.Fatal("CDN connection failed")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("CDN HTTP %d", r.StatusCode)
	}
	d, e := mp3.NewDecoder(io.LimitReader(r.Body, 256<<10))
	if e != nil {
		t.Fatal("Stream is not decodable MP3")
	}
	pcm := make([]byte, 8192)
	n, e := d.Read(pcm)
	if n == 0 || e != nil {
		t.Fatal("MP3 PCM not decoded")
	}
	t.Logf("Authenticated native stream decoded: MP3 %d kbps, %d Hz, %d PCM bytes; track duration %ds, CDN length %d bytes", info.Bitrate, d.SampleRate(), n, track.DurationMS/1000, r.ContentLength)
}
