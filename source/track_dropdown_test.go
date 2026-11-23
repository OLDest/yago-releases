package main

import (
	"context"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
	"testing"
	"time"
	"yago/internal/audio"
	"yago/internal/music"
)

func TestTrackDropdownDoesNotReplaceViewOrClickThrough(t *testing.T) {
	for _, inside := range []bool{true, false} {
		u := &UI{ctx: context.Background(), theme: material.NewTheme(), player: audio.New(t.TempDir(), nil), page: "tracks", title: "Library", current: music.Track{ID: "song", Title: "Song", Artists: []music.Named{{ID: "artist", Name: "Artist"}}}, playID: 17, streamVersion: 9}
		u.theme.Shaper = text.NewShaper(text.WithCollection(uiFonts()))
		u.setTracks([]music.Track{u.current})
		u.actionsMenu()
		var router input.Router
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1600, 829))}
		u.layout(gtx)
		router.Frame(&ops)
		x, y := float32(80), float32(280)
		if inside {
			x, y = 250, float32(829-len(u.disc.entries)*40-47)
		}
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, y)})
		action := u.disc.clicks[0].Clicked(gtx)
		outside := u.disc.dismiss.Clicked(gtx)
		if action != inside || outside == inside || u.rows[0].Clicked(gtx) {
			t.Fatal("dropdown hit routing", inside, action, outside)
		}
		if u.page != "tracks" || u.title != "Library" || u.playID != 17 || u.streamVersion != 9 {
			t.Fatal("dropdown changed library/playback")
		}
		u.player.Close()
	}
}
