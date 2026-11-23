package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
	"yago/internal/music"
	"yago/internal/store"
)

func TestBlockedArtistsCannotPlayOrEnterWave(t *testing.T) {
	old := demo
	demo = true
	defer func() { demo = old }()
	blocked := music.Track{ID: "bad", Artists: []music.Named{{ID: "guest"}, {ID: "blocked"}}}
	good := music.Track{ID: "good"}
	u := &UI{queue: []music.Track{good, blocked, good}, order: []int{0, 1, 2}, index: 0, playID: 12, streamVersion: 7}
	u.disc.prefs = store.Preferences{Artists: map[music.ID]string{"blocked": "Name"}}
	u.startPrepared(1, 0, false, true, nil)
	if u.playID != 12 || u.streamVersion != 7 || u.index != 0 {
		t.Fatal("blocked playback started")
	}
	if u.nextIndex() != 2 {
		t.Fatal("autoplay did not skip blocked artist")
	}
	u.orderPos = 2
	if u.previousIndex() != 0 {
		t.Fatal("previous did not skip block")
	}
	filtered := u.filterRecommendations([]music.Track{blocked, good})
	if len(filtered) != 1 || filtered[0].ID != "good" {
		t.Fatal(filtered)
	}
	u.waveRefreshID = 3
	u.waveTracks = []music.Track{good}
	u.waveRefreshReady(event{waveID: 3, tracks: []music.Track{blocked}, session: "s"})
	if len(u.waveTracks) != 1 || u.waveTracks[0].ID != "good" {
		t.Fatal("empty filtered batch replaced valid cache")
	}
}
func TestDislikePersistsWhenServerFails(t *testing.T) {
	old := stateDir
	stateDir = t.TempDir()
	defer func() { stateDir = old }()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "fail", 500) }))
	defer s.Close()
	api := music.New("")
	api.Base = s.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &UI{ctx: ctx, api: api, account: music.Account{UID: "owner"}, events: make(chan event, 2)}
	u.initPreferences()
	u.moderate(menuEntry{Action: "block-artist", ID: "artist", Name: "Artist"})
	select {
	case e := <-u.events:
		u.handle(e)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	p, e := store.LoadPreferences(stateDir, "owner")
	if e != nil || !p.Blocked(music.Track{Artists: []music.Named{{ID: "artist"}}}) || u.disc.pending {
		t.Fatal(p, e)
	}
}
func TestDiscoveryPagesReplaceAndRejectStaleResults(t *testing.T) {
	u := &UI{gen: 4}
	for page := 0; page < 100; page++ {
		tracks := make([]music.Track, 40)
		for i := range tracks {
			tracks[i] = music.Track{ID: music.ID(fmt.Sprint(page*40 + i))}
		}
		u.exploreReady(event{gen: 4, explore: &exploreEvent{View: exploreView{Kind: "search", Page: page}, Result: music.SearchPage{Tracks: tracks}}})
		if len(u.tracks) != 40 || len(u.rows) != 40 {
			t.Fatal("pages accumulated")
		}
	}
	u.exploreReady(event{gen: 3, explore: &exploreEvent{Result: music.SearchPage{Tracks: []music.Track{{ID: "stale"}}}}})
	if u.tracks[0].ID == "stale" {
		t.Fatal("stale result accepted")
	}
	u.cancelLibrary()
	if u.disc.view.Kind != "" || len(u.disc.result.Albums) != 0 {
		t.Fatal("discovery retained on navigation")
	}
}
func TestWaveSettingsPreservePlayingQueue(t *testing.T) {
	old := stateDir
	stateDir = t.TempDir()
	defer func() { stateDir = old }()
	u := &UI{queue: []music.Track{{ID: "playing"}}, waveTracks: []music.Track{{ID: "cached"}}, playID: 9, requestedOffset: 21}
	u.disc.prefs.Owner = "owner"
	u.menuAction(menuEntry{Action: "wave-option", Group: "diversity", Seed: "settingDiversity:discover"})
	if u.playID != 9 || u.requestedOffset != 21 || u.queue[0].ID != "playing" || u.waveTracks[0].ID != "cached" {
		t.Fatal("settings restarted playback")
	}
	seeds := u.waveSeeds()
	if len(seeds) != 2 || seeds[1] != "settingDiversity:discover" {
		t.Fatal(seeds)
	}
}

func TestNewDiscoveryRequestCancelsOldPage(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("text") == "slow" {
			close(started)
			<-r.Context().Done()
			close(canceled)
			return
		}
		fmt.Fprint(w, `{"result":{"tracks":{"total":1,"perPage":40,"results":[{"id":"latest"}]}}}`)
	}))
	defer s.Close()
	api := music.New("")
	api.Base = s.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &UI{ctx: ctx, api: api, events: make(chan event, 4)}
	u.loadExplore(exploreView{Kind: "search", Query: "slow", Type: "track"})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request not started")
	}
	u.loadExplore(exploreView{Kind: "search", Query: "fast", Type: "track"})
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("request not canceled")
	}
	for i := 0; i < 2; i++ {
		select {
		case e := <-u.events:
			u.handle(e)
		case <-time.After(time.Second):
			t.Fatal("result timeout")
		}
	}
	if len(u.tracks) != 1 || u.tracks[0].ID != "latest" {
		t.Fatal("stale page accepted", u.tracks)
	}
}

func TestDiscoveryDoesNotRetainPreviousPages(t *testing.T) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	u := &UI{gen: 1}
	for page := 0; page < 200; page++ {
		tracks := make([]music.Track, 40)
		for i := range tracks {
			tracks[i] = music.Track{ID: music.ID(fmt.Sprintf("%d-%d", page, i)), Title: strings.Repeat(fmt.Sprintf("Song %d ", page), 100)}
		}
		u.exploreReady(event{gen: 1, explore: &exploreEvent{View: exploreView{Kind: "search", Page: page}, Result: music.SearchPage{Tracks: tracks}}})
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(u)
	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("200 pages: retained heap delta %d bytes", growth)
	if growth > 2<<20 {
		t.Fatal("old pages retained", growth)
	}
}

func TestUnifiedSearchKeepsTracksPlayable(t *testing.T) {
	oldDemo, oldURL := demo, demoURL
	demo, demoURL = true, ""
	defer func() { demo, demoURL = oldDemo, oldURL }()
	u := &UI{gen: 7}
	u.exploreReady(event{gen: 7, explore: &exploreEvent{View: exploreView{Kind: "search", Type: "all"}, Result: music.SearchPage{Tracks: []music.Track{{ID: "song", Title: "Song"}}, Albums: []music.Album{{ID: "album"}}, Artists: []music.Named{{ID: "artist"}}, Playlists: []music.Playlist{{Kind: "playlist"}}}}})
	if u.page != "discovery" || u.exploreCount() != 4 || len(u.rows) != 4 {
		t.Fatal("mixed results lost", u.exploreCount())
	}
	u.openExploreItem(0)
	if u.current.ID != "song" || len(u.queue) != 1 {
		t.Fatal("track result not playable")
	}
}
