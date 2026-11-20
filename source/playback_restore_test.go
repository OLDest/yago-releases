package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"yago/internal/audio"
	"yago/internal/music"
	"yago/internal/store"
)

func playbackFixture(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	f := os.Getenv("YAGO_TEST_MP3")
	if f == "" {
		t.Skip("set YAGO_TEST_MP3 for real audio restart tests")
	}
	b, e := os.ReadFile(f)
	if e != nil {
		t.Fatal(e)
	}
	count := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); w.Write(b) }))
	t.Cleanup(server.Close)
	return server.URL, count
}
func audioReady(t *testing.T, p *audio.Player, check func(audio.Snapshot) bool) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		s := p.Snapshot()
		if s.Error != "" {
			t.Fatal(s.Error)
		}
		if check(s) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("audio timeout", p.Snapshot())
}
func demoPlaybackUI(t *testing.T) *UI {
	t.Helper()
	u := &UI{ctx: context.Background(), volume: 0, muted: true, index: 1, quality: music.Normal}
	u.player = audio.New(filepath.Join(t.TempDir(), "audio"), func() {})
	u.player.Volume(0)
	t.Cleanup(u.player.Close)
	u.queue = []music.Track{{ID: "demo-a", Title: "A", DurationMS: 60000}, {ID: "demo-b", Title: "B", DurationMS: 60000}}
	u.order = []int{1, 0}
	u.shuffled = true
	u.orderPos = 0
	return u
}
func withPlaybackDemo(t *testing.T) {
	t.Helper()
	previousDemo, previousURL, previousState := demo, demoURL, stateDir
	demo = true
	stateDir = t.TempDir()
	t.Cleanup(func() { demo = previousDemo; demoURL = previousURL; stateDir = previousState })
}
func TestBestQualityReloadKeepsPositionAndPause(t *testing.T) {
	withPlaybackDemo(t)
	url, count := playbackFixture(t)
	demoURL = url
	u := demoPlaybackUI(t)
	u.startAt(1, 1, true, false)
	audioReady(t, u.player, func(s audio.Snapshot) bool { return !s.Loading && s.Paused && s.Position >= .99 })
	oldID := u.playID
	before := count.Load()
	u.selectQuality(music.High)
	audioReady(t, u.player, func(s audio.Snapshot) bool { return s.ID != oldID && !s.Loading && s.Paused && s.Position >= .99 })
	if u.quality != music.High || u.index != 1 || u.current.ID != "demo-b" || count.Load() <= before {
		t.Fatal("reload failed to obtain a fresh stream")
	}
	time.Sleep(300 * time.Millisecond)
	if s := u.player.Snapshot(); s.Playing || s.Position > 1.1 {
		t.Fatal("paused reload started playing", s)
	}
}
func TestPlaybackRestartRestoresQueueOffsetAndMode(t *testing.T) {
	for _, playing := range []bool{false, true} {
		name := "paused"
		if playing {
			name = "playing"
		}
		t.Run(name, func(t *testing.T) {
			withPlaybackDemo(t)
			url, _ := playbackFixture(t)
			demoURL = url
			u := demoPlaybackUI(t)
			u.startAt(1, .75, !playing, false)
			audioReady(t, u.player, func(s audio.Snapshot) bool { return !s.Loading && s.Position >= .74 && s.Playing == playing })
			u.savePlayback(true)
			saved, e := store.LoadPlayback(stateDir)
			if e != nil {
				t.Fatal(e)
			}
			// Both instances live only during this bounded test; pause the first.
			u.player.Pause()
			v := demoPlaybackUI(t)
			v.pendingPlayback = &saved
			v.restorePlayback()
			audioReady(t, v.player, func(s audio.Snapshot) bool {
				return !s.Loading && s.Position >= saved.Position-.05 && s.Playing == playing
			})
			if v.current.ID != "demo-b" || v.index != 1 || v.order[0] != 1 || !v.shuffled || !v.muted || v.volume != 0 || v.pauseRequested == playing {
				t.Fatal("checkpoint state not restored")
			}
		})
	}
}
func TestCheckpointWaitsForCorrectAccount(t *testing.T) {
	u := &UI{pendingPlayback: &store.Playback{Owner: "42", Queue: []music.Track{{ID: "a"}}}}
	u.restorePlayback()
	if u.pendingPlayback == nil {
		t.Fatal("checkpoint discarded before login")
	}
	u.api = music.New("")
	u.account.UID = "99"
	u.restorePlayback()
	if len(u.queue) != 0 || u.current.ID != "" {
		t.Fatal("another account queue restored")
	}
}
func TestStoppedCheckpointDoesNotAutoplay(t *testing.T) {
	withPlaybackDemo(t)
	url, _ := playbackFixture(t)
	demoURL = url
	u := demoPlaybackUI(t)
	u.startAt(1, 0, true, false)
	audioReady(t, u.player, func(s audio.Snapshot) bool { return !s.Loading && s.Paused })
	u.mediaCommand("stop")
	saved, e := store.LoadPlayback(stateDir)
	if e != nil || !saved.Stopped || saved.Playing {
		t.Fatal(saved, e)
	}
	v := demoPlaybackUI(t)
	v.pendingPlayback = &saved
	v.restorePlayback()
	if v.player.Snapshot().ID != 0 || !v.stopped || v.current.ID != "demo-b" {
		t.Fatal("stopped checkpoint started audio")
	}
}

func TestTaskRemovalDoesNotOverwriteSavedPosition(t *testing.T) {
	withPlaybackDemo(t)
	url, _ := playbackFixture(t)
	demoURL = url
	u := demoPlaybackUI(t)
	u.startAt(1, .75, false, false)
	audioReady(t, u.player, func(s audio.Snapshot) bool { return !s.Loading && s.Playing && s.Position >= .74 })
	u.mediaCommand("task-removed")
	u.savePlayback(true)
	p, e := store.LoadPlayback(stateDir)
	if e != nil || !p.Playing || p.Stopped || p.Position < .74 {
		t.Fatalf("task removal damaged checkpoint: %+v %v", p, e)
	}
	u.mediaCommand("play")
	audioReady(t, u.player, func(s audio.Snapshot) bool { return !s.Loading && s.Playing && s.Position >= p.Position-.05 })
}
