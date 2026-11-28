package main

import (
	"context"
	"strings"
	"testing"
	"time"
	"yago/internal/music"
)

type artistSearchProbe struct {
	music.Backend
	queries chan string
}

func (p *artistSearchProbe) SearchPage(_ context.Context, q, _ string, _ int) (music.SearchPage, error) {
	p.queries <- q
	return music.SearchPage{}, nil
}

func TestArtistSearchReturnsToCurrentWave(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probe := &artistSearchProbe{queries: make(chan string, 1)}
	playing := music.Track{ID: "playing", Artists: []music.Named{{Name: " Artist One "}, {Name: "Artist Two"}}}
	u := &UI{
		ctx: ctx, events: make(chan event, 1), api: &music.API{Backend: probe},
		section: sectionWave, page: "tracks", title: "Моя волна", viewWave: true, isWave: true,
		queue: []music.Track{playing, {ID: "next"}}, waveTracks: []music.Track{playing, {ID: "next"}},
		current: playing, index: 0, playID: 73, requestedOffset: 42,
		session: "session", batch: "batch",
	}
	u.actionsMenu()
	var action menuEntry
	for _, entry := range u.disc.entries {
		if entry.Action == "artist-search" && entry.Name == "Artist Two" {
			action = entry
		}
	}
	if action.Label != "Поиск по исполнителю: Artist Two" {
		t.Fatal("artist action missing", action)
	}
	u.menuAction(action)
	if u.editor.Text() != "Artist Two" || u.disc.view.Query != "Artist Two" || u.section != sectionExplore || u.viewWave {
		t.Fatal("artist search did not open the shared search view")
	}
	select {
	case q := <-probe.queries:
		if q != "Artist Two" {
			t.Fatal("wrong artist query", q)
		}
	case <-time.After(time.Second):
		t.Fatal("search request not sent")
	}
	select {
	case result := <-u.events:
		u.handle(result)
	case <-time.After(time.Second):
		t.Fatal("empty search result not delivered")
	}
	if u.status != "Ничего не найдено" || u.current.ID != "playing" || u.queue[1].ID != "next" || u.playID != 73 || u.requestedOffset != 42 || u.session != "session" || u.batch != "batch" {
		t.Fatal("empty search changed Wave playback")
	}
	u.openWave()
	if u.page != "tracks" || u.title != "Моя волна" || u.section != sectionWave || !u.viewWave || !u.isWave || u.busy || u.tracks[0].ID != "playing" || u.current.ID != "playing" || u.playID != 73 || u.requestedOffset != 42 {
		t.Fatal("returning to Wave restarted playback")
	}

	u.menuAction(action)
	searchGen := u.gen
	u.openWave() // Cancels a pending search without replacing the Wave.
	u.handle(event{kind: "explore", gen: searchGen, explore: &exploreEvent{View: exploreView{Kind: "search"}, Result: music.SearchPage{Tracks: []music.Track{{ID: "stale"}}}}})
	if u.title != "Моя волна" || u.tracks[0].ID != "playing" || u.queue[0].ID != "playing" || u.playID != 73 || u.session != "session" {
		t.Fatal("canceled search replaced the Wave")
	}
}

func TestArtistSearchWithoutArtistExplainsMissingMetadata(t *testing.T) {
	u := &UI{current: music.Track{ID: "playing"}}
	u.actionsMenu()
	for _, entry := range u.disc.entries {
		if entry.Action == "artist-search-missing" {
			u.menuAction(entry)
			if !strings.Contains(u.status, "не указан исполнитель") {
				t.Fatal("missing artist message", u.status)
			}
			return
		}
	}
	t.Fatal("missing artist action not shown")
}
