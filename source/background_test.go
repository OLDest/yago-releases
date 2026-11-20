package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"yago/internal/audio"
	"yago/internal/music"
)

func TestQueueAdvancesWithoutWindowFrameAndIgnoresStaleEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := audio.New(t.TempDir(), nil)
	defer p.Close()
	api := music.New("")
	api.Base = server.URL
	u := &UI{ctx: ctx, api: api, player: p, queue: []music.Track{{ID: "one"}, {ID: "two"}, {ID: "three"}}, index: 0, current: music.Track{ID: "one"}, playID: 99}
	u.advanceIfEnded(audio.Snapshot{ID: 99, Ended: true})
	if u.index != 1 || u.current.ID != "two" {
		t.Fatal("background end did not advance queue", u.index)
	}
	u.advanceIfEnded(audio.Snapshot{ID: 99, Ended: true})
	if u.index != 1 {
		t.Fatal("stale completion skipped another track")
	}
}
