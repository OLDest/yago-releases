package main

import (
	"context"
	"errors"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
	"yago/internal/music"
)

func TestWaveRefreshChangesOnlyList(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"radioSessionId":"fresh","batchId":"fresh-batch","sequence":[{"track":{"id":"new","title":"New"}}]}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := music.New("")
	api.Base = server.URL
	old := music.Track{ID: "playing"}
	u := &UI{ctx: ctx, api: api, events: make(chan event, 8), page: "tracks", viewWave: true, isWave: true, title: "Моя волна", current: old, queue: []music.Track{old}, waveTracks: []music.Track{old}, session: "old-session", batch: "old-batch", playID: 17, streamVersion: 21, pauseRequested: true, requestedOffset: 42, index: 0, order: []int{0}, waveViewSession: "old-session"}
	u.setTracks(u.waveTracks)
	u.refreshWave()
	u.refreshWave()
	select {
	case e := <-u.events:
		u.handle(e)
	case <-time.After(3 * time.Second):
		t.Fatal("refresh timed out")
	}
	if requests.Load() != 1 || u.waveRefreshing || u.tracks[0].ID != "new" {
		t.Fatal("refresh did not update list", requests.Load())
	}
	if u.playID != 17 || u.streamVersion != 21 || u.current.ID != "playing" || u.queue[0].ID != "playing" || u.session != "old-session" || u.batch != "old-batch" || !u.pauseRequested || u.requestedOffset != 42 {
		t.Fatal("refresh changed playback")
	}
	u.openOffline()
	u.openWave()
	if u.tracks[0].ID != "new" {
		t.Fatal("returning to Wave lost refreshed list")
	}
	u.api = nil
	u.activateWaveView()
	if u.session != "fresh" || u.batch != "fresh-batch" || u.waveViewRefreshed {
		t.Fatal("track selection did not activate refreshed session")
	}
}
func TestFailedWaveRefreshKeepsPreviousList(t *testing.T) {
	u := &UI{waveRefreshID: 1, waveRefreshing: true, waveTracks: []music.Track{{ID: "old"}}, session: "playing-session", playID: 9}
	u.waveRefreshReady(event{waveID: 1, err: errors.New("no network")})
	if u.waveTracks[0].ID != "old" || u.playID != 9 || u.session != "playing-session" || u.waveRefreshing {
		t.Fatal("failed refresh changed state")
	}
	u.waveRefreshReady(event{waveID: 1, tracks: []music.Track{{ID: "stale"}}, session: "stale"})
	if u.waveTracks[0].ID != "old" {
		t.Fatal("stale refresh applied")
	}
}
func TestOldWaveContinuationDoesNotReplaceRefreshedList(t *testing.T) {
	oldDemo, oldURL := demo, demoURL
	demo, demoURL = true, ""
	defer func() { demo, demoURL = oldDemo, oldURL }()
	u := &UI{isWave: true, viewWave: true, page: "tracks", title: "Моя волна", session: "old", waveNextID: 1, streamVersion: 7, waveViewRefreshed: true, waveViewSession: "fresh", waveTracks: []music.Track{{ID: "refreshed"}}, pauseRequested: true}
	u.setTracks(u.waveTracks)
	u.waveContinuationReady(event{waveID: 1, streamID: 7, tracks: []music.Track{{ID: "continuation"}}, session: "old", batch: "batch2"})
	if u.tracks[0].ID != "refreshed" || u.waveTracks[0].ID != "refreshed" || u.queue[0].ID != "continuation" || !u.pauseRequested {
		t.Fatal("continuation overwrote refreshed view")
	}
}
func TestWaveRefreshButtonHitTarget(t *testing.T) {
	for _, width := range []int{320, 800} {
		u := &UI{theme: material.NewTheme(), viewWave: true, page: "tracks", title: "Моя волна"}
		u.theme.Shaper = text.NewShaper(text.WithCollection(uiFonts()))
		var router input.Router
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Now(), Constraints: layout.Constraints{Max: image.Pt(width, 60)}}
		d := u.viewHeading(gtx, 18)
		if d.Size.X > width || d.Size.Y > 60 {
			t.Fatal("heading overflow", d.Size)
		}
		router.Frame(&ops)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(float32(width-30), 20)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(float32(width-30), 20)})
		if !u.waveRefresh.Clicked(gtx) {
			t.Fatal("refresh button not clickable", width)
		}
	}
}
