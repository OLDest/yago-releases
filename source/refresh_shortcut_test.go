package main

import (
	"context"
	gioevent "gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
	"yago/internal/music"
)

func TestF5RefreshWithEditorFocusSuppressesRepeat(t *testing.T) {
	var requests atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"radioSessionId":"fresh","sequence":[{"track":{"id":"new"}}]}`))
	}))
	defer s.Close()
	api := music.New("")
	api.Base = s.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &UI{ctx: ctx, api: api, events: make(chan event, 2), viewWave: true, page: "tracks", playID: 17, current: music.Track{ID: "playing"}}
	var router input.Router
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Source: router.Source()}
	gioevent.Op(&ops, &u.editor)
	u.refreshShortcut(gtx)
	router.Frame(&ops)
	gtx.Execute(key.FocusCmd{Tag: &u.editor})
	u.refreshShortcut(gtx)
	router.Frame(&ops)
	router.Queue(key.Event{Name: key.NameF5, State: key.Press}, key.Event{Name: key.NameF5, State: key.Release}, key.Event{Name: key.NameF5, State: key.Press})
	u.refreshShortcut(gtx)
	select {
	case e := <-u.events:
		u.handle(e)
	case <-time.After(time.Second):
		t.Fatal("F5 refresh timed out")
	}
	if requests.Load() != 1 || u.playID != 17 || u.current.ID != "playing" || len(u.waveTracks) != 1 {
		t.Fatal("F5 did not use guarded refresh", requests.Load())
	}
}
func TestF5DoesNotRefreshOtherViews(t *testing.T) {
	for _, mode := range []string{"search", "menu", "offline", "updates"} {
		u := &UI{viewWave: true, page: "tracks"}
		switch mode {
		case "search":
			u.viewWave = false
		case "menu":
			u.disc.menu = "actions"
		case "offline":
			u.off.picking = true
		case "updates":
			u.up.show = true
		}
		var router input.Router
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Source: router.Source()}
		u.refreshShortcut(gtx)
		router.Frame(&ops)
		router.Queue(key.Event{Name: key.NameF5, State: key.Press})
		u.refreshShortcut(gtx)
		if u.waveRefreshing || u.status != "" {
			t.Fatal("F5 active in hidden view", mode)
		}
	}
}
