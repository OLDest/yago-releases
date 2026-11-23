package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
	"yago/internal/audio"
	"yago/internal/music"
	"yago/internal/store"
)

func TestLiveWaveFetch15(t *testing.T) {
	state := os.Getenv("YAGO_LIVE_WAVE_STATE")
	if state == "" {
		t.Skip("explicit saved-account Wave verification")
	}
	device, e := os.ReadFile(filepath.Join(state, "device-id"))
	if e != nil || len(device) != 32 {
		t.Fatal("device unavailable")
	}
	token, e := store.Load(string(device))
	if e != nil {
		t.Fatal("Yandex account unavailable")
	}
	api := music.New(token.Access)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	account, e := api.Account(ctx)
	if e != nil {
		t.Fatal(e)
	}
	prefs, e := store.LoadPreferences(state, account.UID)
	if e != nil {
		t.Fatal(e)
	}
	tracks, contexts, e := fetchWaveTail(ctx, api, "", nil, nil, prefs)
	if e != nil {
		t.Fatal(e)
	}
	if len(tracks) != 15 || len(contexts) != 15 {
		t.Fatal("not 15 tracks")
	}
	seen := map[music.ID]bool{}
	for i, tr := range tracks {
		if seen[tr.ID] || contexts[i].Session == "" || prefs.Blocked(tr) {
			t.Fatal("invalid tail")
		}
		seen[tr.ID] = true
	}
	t.Log("Live Yandex Wave: fetched 15 unique allowed tracks with per-track session context")
}

// Opt-in account check: keep one real stream alive until its natural end,
// then resolve the next track. Do not print signed URLs or account tokens.
func TestLiveWaveSequentialPlayback(t *testing.T) {
	state := os.Getenv("YAGO_LIVE_SEQUENTIAL_STATE")
	if state == "" {
		t.Skip("explicit saved-account sequential playback check")
	}
	device, err := os.ReadFile(filepath.Join(state, "device-id"))
	if err != nil {
		t.Fatal("device unavailable")
	}
	token, err := store.Load(string(device))
	if err != nil {
		t.Fatal("account unavailable")
	}
	a := music.New(token.Access)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	account, err := a.Account(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := store.LoadPreferences(state, account.UID)
	if err != nil {
		t.Fatal(err)
	}
	tracks, _, err := fetchWaveTail(ctx, a, "", nil, nil, prefs)
	if err != nil || len(tracks) < 2 {
		t.Fatal("wave unavailable", err)
	}
	p := audio.New(t.TempDir(), nil)
	defer p.Close()
	p.Volume(0)
	for i := 0; i < 2; i++ {
		track := tracks[i]
		id := p.Play(func(ctx context.Context) (string, error) {
			info, e := a.Stream(ctx, track, music.High)
			return info.URL, e
		})
		started := false
		ticker := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				t.Fatal("sequential playback timeout")
			case <-ticker.C:
			}
			s := p.Snapshot()
			if s.ID != id {
				continue
			}
			if s.Error != "" {
				ticker.Stop()
				t.Fatalf("track %d after %.1fs: %s", i+1, s.Position, s.Error)
			}
			if s.Playing && !started {
				t.Logf("track %d started", i+1)
				started = true
			}
			if (i == 0 && s.Ended) || (i == 1 && s.Position >= 10) {
				t.Logf("track %d passed at %.1fs; ended=%v", i+1, s.Position, s.Ended)
				break
			}
		}
		ticker.Stop()
	}
}

func TestLiveWaveDelayedStreams(t *testing.T) {
	state := os.Getenv("YAGO_LIVE_DELAYED_STATE")
	if state == "" {
		t.Skip("explicit account and delayed CDN check")
	}
	device, err := os.ReadFile(filepath.Join(state, "device-id"))
	if err != nil {
		t.Fatal("device unavailable")
	}
	token, err := store.Load(string(device))
	if err != nil {
		t.Fatal("account unavailable")
	}
	a := music.New(token.Access)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	account, err := a.Account(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := store.LoadPreferences(state, account.UID)
	if err != nil {
		t.Fatal(err)
	}
	tracks, _, err := fetchWaveTail(ctx, a, "", nil, nil, prefs)
	if err != nil || len(tracks) < 2 {
		t.Fatal("wave unavailable", err)
	}
	q := music.High
	if b, e := os.ReadFile(filepath.Join(state, "quality")); e == nil && string(b) == "192" {
		q = music.Normal
	}
	read := func(raw string) {
		t.Helper()
		s, e := audio.OpenStream(ctx, raw, t.TempDir())
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		if _, e = io.CopyN(io.Discard, s.Reader(ctx), 64<<10); e != nil {
			t.Fatal("CDN audio read", e)
		}
	}
	var old string
	for i := 0; i < 2; i++ {
		info, e := a.Stream(ctx, tracks[i], q)
		if e != nil {
			t.Fatalf("initial track %d: %v", i+1, e)
		}
		read(info.URL)
		old = info.URL
	}
	t.Log("initial two streams resolved and read; waiting 90 seconds")
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(90 * time.Second):
	}
	// The application normally resolves fresh at playback start. Check both
	// that path and the older link without logging either signed address.
	info, err := a.Stream(ctx, tracks[1], q)
	if err != nil {
		t.Fatal("delayed resolver", err)
	}
	read(info.URL)
	t.Log("fresh stream after 90 seconds: 64 KiB read")
	read(old)
	t.Log("original stream after 90 seconds: 64 KiB read")
}
