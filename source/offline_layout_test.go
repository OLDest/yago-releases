package main

import (
	"context"
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

func TestOfflinePhonePanelsFitWithoutNativeWindow(t *testing.T) {
	old := stateDir
	stateDir = t.TempDir()
	defer func() { stateDir = old }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &UI{ctx: ctx, theme: material.NewTheme(), player: audio.New(t.TempDir(), nil), current: music.Track{ID: "test", Title: "Локальная музыка", DurationMS: 120000}, title: "Оффлайн"}
	defer u.player.Close()
	u.theme.Shaper = text.NewShaper(text.WithCollection(uiFonts()))
	u.initOffline()
	defer u.off.manager.Close()
	u.list.Axis = layout.Vertical
	for _, size := range []image.Point{image.Pt(380, 820), image.Pt(320, 640)} {
		for _, panel := range []string{"library", "picker", "remove"} {
			u.page = "offline"
			u.off.picking = false
			u.off.removing = ""
			if panel == "picker" {
				u.page = "offline-tracks"
				u.off.picking = true
			}
			if panel == "remove" {
				u.page = "offline-tracks"
				u.off.removing = "test"
			}
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
			d := u.layout(gtx)
			if d.Size.X > size.X || d.Size.Y > size.Y || d.Size.X < 0 || d.Size.Y < 0 {
				t.Fatalf("%s %v: panel overflow %v", panel, size, d.Size)
			}
		}
	}
}
