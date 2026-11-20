package main

import (
	"context"
	"errors"
	"fmt"
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
	"yago/internal/audio"
	"yago/internal/music"
)

func TestRowActionDoesNotClickPlayback(t *testing.T) {
	for _, page := range []string{"tracks", "offline-tracks", "discovery"} {
		for _, source := range []pointer.Source{pointer.Mouse, pointer.Touch} {
			for _, width := range []int{320, 800} {
				for _, action := range []bool{true, false} {
					u := &UI{theme: material.NewTheme(), page: page}
					u.theme.Shaper = text.NewShaper(text.WithCollection(uiFonts()))
					u.list.Axis = layout.Vertical
					u.setTracks([]music.Track{{ID: "one", Title: "A song", DurationMS: 180000}})
					u.disc.result.Tracks = u.tracks
					var router input.Router
					var ops op.Ops
					gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, 62))}
					if page == "discovery" {
						u.exploreRow(gtx, 0)
					} else {
						u.content(gtx)
					}
					router.Frame(&ops)
					x := float32(80)
					if action {
						x = float32(width - 30)
					}
					buttons := pointer.ButtonPrimary
					if source == pointer.Touch {
						buttons = 0
					}
					router.Queue(pointer.Event{Kind: pointer.Press, Source: source, PointerID: 1, Buttons: buttons, Position: f32.Pt(x, 31)}, pointer.Event{Kind: pointer.Release, Source: source, PointerID: 1, Position: f32.Pt(x, 31)})
					clickedAction := u.off.actions[0].Clicked(gtx)
					clickedPlay := u.rows[0].Clicked(gtx)
					if clickedAction != action || clickedPlay == action {
						t.Fatalf("page=%s source=%v width=%d action=%v: action click=%v play click=%v", page, source, width, action, clickedAction, clickedPlay)
					}
				}
			}
		}
	}
}

func TestReturningToWaveDoesNotRestartOrReplacePlayback(t *testing.T) {
	for _, activeWave := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		active := []music.Track{{ID: "playing"}, {ID: "next"}}
		cached := []music.Track{{ID: "wave-cached"}}
		u := &UI{ctx: ctx, page: "offline", waveTracks: cached, queue: active, current: active[0], index: 0, isWave: activeWave, queueOffline: !activeWave, offlineQueuePlaylist: "local", playID: 73, streamVersion: 11, requestedOffset: 42, pauseRequested: true, session: "session", batch: "batch", order: []int{1, 0}, orderPos: 1, shuffled: true}
		u.openWave()
		if u.playID != 73 || u.streamVersion != 11 || u.requestedOffset != 42 || !u.pauseRequested || u.current.ID != "playing" || u.index != 0 || u.orderPos != 1 || u.queue[0].ID != "playing" || u.queueOffline == activeWave || u.busy {
			t.Fatal("navigation changed active playback", activeWave)
		}
		want := music.ID("wave-cached")
		if activeWave {
			want = "playing"
		}
		if u.page != "tracks" || u.title != "Моя волна" || u.tracks[0].ID != want {
			t.Fatal("cached Wave view missing", activeWave)
		}
		u.openWave()
		if u.playID != 73 || u.streamVersion != 11 {
			t.Fatal("repeated navigation restarted playback")
		}
		cancel()
	}
}
func TestFailedWaveRequestPreservesOfflineQueue(t *testing.T) {
	u := &UI{queueOffline: true, offlineQueuePlaylist: "local", queue: []music.Track{{ID: "local-track"}}}
	u.handle(event{kind: "wave", err: errors.New("network unavailable")})
	if !u.queueOffline || u.offlineQueuePlaylist != "local" || u.queue[0].ID != "local-track" {
		t.Fatal("failed Wave changed playback source")
	}
}
func TestOpeningWaveDoesNotCancelAutomaticContinuation(t *testing.T) {
	canceled := false
	u := &UI{isWave: true, queue: []music.Track{{ID: "playing"}}, title: "Моя волна", busy: true, advancePending: true, gen: 19, playID: 7, libraryCancel: func() { canceled = true }}
	u.openWave()
	if canceled || u.gen != 19 || !u.busy || !u.advancePending || u.playID != 7 {
		t.Fatal("automatic continuation canceled")
	}
}
func TestLeavingLibraryRejectsAlreadyQueuedWaveResult(t *testing.T) {
	u := &UI{gen: 10, libraryCancel: func() {}, queueOffline: true, queue: []music.Track{{ID: "local"}}}
	u.openOffline()
	u.handle(event{kind: "wave", gen: 10, tracks: []music.Track{{ID: "stale"}}, session: "old"})
	if u.page != "offline" || u.queue[0].ID != "local" || !u.queueOffline {
		t.Fatal("stale Wave response took over playback")
	}
}
func TestWaveContinuationDoesNotTakeOverBrowsedPlaylist(t *testing.T) {
	oldDemo, oldURL := demo, demoURL
	demo, demoURL = true, ""
	defer func() { demo, demoURL = oldDemo, oldURL }()
	canceled := false
	u := &UI{isWave: true, queue: []music.Track{{ID: "old"}}, current: music.Track{ID: "old"}, waveNextID: 1, streamVersion: 40, waveNextCancel: func() { canceled = true }, advancePending: true}
	u.openOffline()
	if canceled {
		t.Fatal("navigation canceled continuation")
	}
	u.handle(event{kind: "wave-next", waveID: 1, streamID: 40, tracks: []music.Track{{ID: "new"}}, session: "session", batch: "batch"})
	if u.page != "offline" || u.title != "Оффлайн-плейлисты" || len(u.queue) != 2 || u.queue[0].ID != "old" || u.queue[1].ID != "new" || u.waveTracks[1].ID != "new" {
		t.Fatal("continuation took over the visible playlist")
	}
}
func TestStoppedWaveRejectsQueuedContinuation(t *testing.T) {
	p := audio.New(t.TempDir(), nil)
	defer p.Close()
	u := &UI{player: p, isWave: true, queue: []music.Track{{ID: "old"}}, current: music.Track{ID: "old"}, waveNextID: 1, streamVersion: 40, waveNextCancel: func() {}}
	u.mediaCommand("stop")
	u.handle(event{kind: "wave-next", waveID: 1, streamID: 40, tracks: []music.Track{{ID: "new"}}})
	if !u.stopped || u.queue[0].ID != "old" {
		t.Fatal("stale continuation restarted stopped Wave")
	}
}
func TestWaveContinuationRequestSurvivesNavigationAndIsReused(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		batch := requests.Add(1) - 1
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"radioSessionId":"session","batchId":"batch2","sequence":[`)
		for i := 0; i < 5; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"track":{"id":"next-%d","title":"Next"}}`, batch*5+int32(i))
		}
		fmt.Fprint(w, `]}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldDemo, oldURL := demo, demoURL
	demo, demoURL = true, ""
	defer func() { demo, demoURL = oldDemo, oldURL }()
	api := music.New("")
	api.Base = server.URL
	u := &UI{ctx: ctx, events: make(chan event, 8), api: api, isWave: true, queue: []music.Track{{ID: "old"}}, current: music.Track{ID: "old"}, session: "session", streamVersion: 5}
	u.continueWave()
	u.openOffline()
	select {
	case e := <-u.events:
		u.handle(e)
	case <-time.After(3 * time.Second):
		t.Fatal("continuation canceled by navigation")
	}
	if u.page != "offline" || len(u.queue) != 16 || u.queue[1].ID != "next-0" {
		t.Fatal("continuation changed view or failed", u.status)
	}
	for i := 0; i < 3; i++ {
		u.openWave()
		u.openOffline()
	}
	if requests.Load() != 3 || u.busy {
		t.Fatal("returning to Wave issued another request", requests.Load())
	}
}
func TestWaveContinuationKeepsPauseRequestedWhileWaiting(t *testing.T) {
	oldDemo, oldURL := demo, demoURL
	demo, demoURL = true, ""
	defer func() { demo, demoURL = oldDemo, oldURL }()
	u := &UI{isWave: true, waveNextID: 1, streamVersion: 7, pauseRequested: true}
	u.handle(event{kind: "wave-next", waveID: 1, streamID: 7, tracks: []music.Track{{ID: "next"}}, session: "session"})
	if !u.pauseRequested {
		t.Fatal("new Wave batch cleared pause")
	}
}
