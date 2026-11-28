package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
	"yago/internal/audio"
	"yago/internal/music"
	"yago/internal/offline"
	"yago/internal/store"
)

func TestOfflinePlaybackAndRestartWithoutAccountOrServer(t *testing.T) {
	fixture := os.Getenv("YAGO_TEST_MP3")
	if fixture == "" {
		t.Skip("YAGO_TEST_MP3 required")
	}
	f, e := os.Open(fixture)
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(io.LimitReader(f, 128<<10))
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	previousState, previousDemo := stateDir, demo
	stateDir = t.TempDir()
	demo = false
	defer func() { stateDir = previousState; demo = previousDemo }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	library, e := offline.Open(filepath.Join(stateDir, "offline"))
	if e != nil {
		t.Fatal(e)
	}
	playlist, e := library.Create("Дорога")
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	manager := offline.NewManager(ctx, library, nil)
	track := music.Track{ID: "offline-test", Title: "Local song", DurationMS: 120000}
	manager.Enqueue([]offline.Job{{Track: track, Playlist: playlist, Quality: music.High, Resolve: func(context.Context, music.Track, music.Quality) (music.StreamInfo, error) {
		return music.StreamInfo{URL: server.URL, Bitrate: 192}, nil
	}}})
	until := time.Now().Add(8 * time.Second)
	for manager.Snapshot().Done == 0 && time.Now().Before(until) {
		if p := manager.Snapshot(); p.Failed > 0 {
			t.Fatal(p)
		}
		time.Sleep(10 * time.Millisecond)
	}
	manager.Close()
	server.Close()
	path, _, e := library.Resolve(track.ID)
	if e != nil {
		t.Fatal(e)
	}
	wait := func(p *audio.Player, paused bool) {
		t.Helper()
		until := time.Now().Add(8 * time.Second)
		for time.Now().Before(until) {
			s := p.Snapshot()
			if s.Error != "" {
				t.Fatal(s.Error)
			}
			if s.ID != 0 && !s.Loading && s.Paused == paused && s.Position >= .2 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("local playback timed out", p.Snapshot())
	}
	u := &UI{ctx: ctx, player: audio.New(t.TempDir(), nil), off: offlineUI{library: library, view: playlist}, queue: []music.Track{track}, queueOffline: true, offlineQueuePlaylist: playlist, index: 0, quality: music.High}
	u.startAt(0, .25, true, false)
	wait(u.player, true)
	u.savePlayback(true)
	u.player.Close()
	saved, e := store.LoadPlayback(stateDir)
	if e != nil || !saved.Offline || saved.Owner != "__offline__" || saved.OfflinePlaylist != playlist || saved.Playing {
		t.Fatal(saved, e)
	}
	v := &UI{ctx: ctx, player: audio.New(t.TempDir(), nil), off: offlineUI{library: library}, viewWave: true}
	defer v.player.Close()
	v.initPlaybackRestore()
	wait(v.player, true)
	if v.api != nil || !v.queueOffline || v.off.view != playlist || v.viewWave {
		t.Fatal("offline checkpoint required account")
	}
	id := v.playID
	v.selectQuality(music.Normal)
	if v.playID != id || v.quality != music.Normal {
		t.Fatal("quality change restarted cached audio")
	}
	v.mediaCommand("play")
	wait(v.player, false)
	v.player.Stop()
	if _, e = os.Stat(path); e != nil {
		t.Fatal("persistent audio removed", e)
	}
	// Deleting an ended/stopped current entry must not start its successor.
	w := &UI{ctx: ctx, player: audio.New(t.TempDir(), nil), off: offlineUI{library: library, view: playlist}, queueOffline: true, offlineQueuePlaylist: playlist, queue: []music.Track{{ID: "removed"}, track}, current: music.Track{ID: "removed"}, index: 0}
	defer w.player.Close()
	w.player.Volume(0)
	w.removeFromOfflineQueue("removed")
	until = time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		s := w.player.Snapshot()
		if s.Error != "" {
			t.Fatal(s.Error)
		}
		if s.ID != 0 && !s.Loading {
			if !s.Paused || s.Playing {
				t.Fatal("deletion started playback", s)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("paused successor did not load")
}
func TestOfflineCheckpointRejectsAccountImpersonation(t *testing.T) {
	p := store.Playback{Version: 1, Owner: "account-42", Offline: true, Queue: []music.Track{{ID: "one"}}, Order: []int{0}, Index: 0}
	if e := p.Validate(); e == nil {
		t.Fatal("offline flag bypassed owner validation")
	}
	p.Owner = "__offline__"
	p.Demo = true
	if e := p.Validate(); e == nil {
		t.Fatal("demo marked as offline")
	}
}
func TestRemovingOfflineQueueEntryPreservesCurrentTrackIndex(t *testing.T) {
	l, e := offline.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	u := &UI{off: offlineUI{library: l, view: offline.DefaultID}, queueOffline: true, offlineQueuePlaylist: offline.DefaultID, queue: []music.Track{{ID: "a"}, {ID: "b"}, {ID: "c"}}, current: music.Track{ID: "c"}, index: 2}
	u.removeFromOfflineQueue("b")
	if len(u.queue) != 2 || u.index != 1 || u.queue[u.index].ID != "c" {
		t.Fatal("queue index lost", u.index, u.queue)
	}
}
