package main

import (
	"gioui.org/f32"
	gioevent "gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"image"
	"testing"
	"time"
	"yago/internal/music"
)

func TestListSwipeRouting(t *testing.T) {
	for _, tc := range []struct {
		name         string
		source       pointer.Source
		dx, dy       float32
		want         int
		tap, enabled bool
	}{
		{"left", pointer.Touch, -100, 4, 1, false, true},
		{"right", pointer.Touch, 100, 4, -1, false, true},
		{"tap", pointer.Touch, 0, 0, 0, true, true},
		{"short", pointer.Touch, 30, 0, 0, false, true},
		{"vertical", pointer.Touch, 10, 100, 0, false, true},
		{"diagonal", pointer.Touch, 100, 100, 0, false, true},
		{"mouse", pointer.Mouse, -100, 0, 0, false, true},
		{"disabled", pointer.Touch, -100, 0, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var router input.Router
			var ops op.Ops
			var swipe listSwipe
			var row widget.Clickable
			clicks, direction := 0, 0
			frame := func(events ...gioevent.Event) {
				router.Queue(events...)
				ops.Reset()
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 200))}
				direction += swipe.update(gtx, tc.enabled)
				if row.Clicked(gtx) {
					clicks++
				}
				stack := clip.Rect{Max: gtx.Constraints.Max}.Push(&ops)
				row.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
				pass := pointer.PassOp{}.Push(&ops)
				gioevent.Op(&ops, &swipe)
				pass.Pop()
				stack.Pop()
				router.Frame(&ops)
			}
			frame()
			frame(pointer.Event{Kind: pointer.Press, Source: tc.source, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(180, 50)})
			frame(pointer.Event{Kind: pointer.Move, Source: tc.source, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(180+tc.dx, 50+tc.dy)})
			frame(pointer.Event{Kind: pointer.Release, Source: tc.source, PointerID: 1, Position: f32.Pt(180+tc.dx, 50+tc.dy)})
			frame()
			if direction != tc.want {
				t.Fatalf("direction %d, want %d", direction, tc.want)
			}
			if tc.want != 0 && clicks != 0 {
				t.Fatal("swipe clicked underlying track")
			}
			if tc.tap && clicks != 1 {
				t.Fatalf("tap lost: %d", clicks)
			}
		})
	}
}

func TestSwipeModalAndDesktopGuards(t *testing.T) {
	previous := windowMode
	defer func() { windowMode = previous }()
	windowMode = "phone"
	u := &UI{page: "tracks"}
	if !u.swipeEnabled() {
		t.Fatal("phone list disabled")
	}
	u.disc.menu = "actions"
	if u.swipeEnabled() {
		t.Fatal("swipe under track menu")
	}
	u.disc.menu = ""
	u.qualityMenuOpen = true
	if u.swipeEnabled() {
		t.Fatal("swipe under quality menu")
	}
	u.qualityMenuOpen = false
	u.page = "home"
	if u.swipeEnabled() {
		t.Fatal("swipe on login screen")
	}
}

func TestSwipePagesStayInsideSectionAndKeepPlayback(t *testing.T) {
	for _, section := range []librarySection{sectionWave, sectionLikes, sectionPlaylists, sectionOffline} {
		page := "tracks"
		if section == sectionPlaylists {
			page = "playlists"
		}
		if section == sectionOffline {
			page = "offline-tracks"
		}
		u := &UI{section: section, page: page, title: "Current list", index: 1, playID: 23, current: music.Track{ID: "playing"}, viewWave: section == sectionWave, fit: fitUI{visible: 5, count: 12}}
		for _, want := range []int{5, 10, 10} {
			u.turnListPage(1)
			if u.fit.offset != want {
				t.Fatalf("next page offset %d, want %d", u.fit.offset, want)
			}
		}
		for _, want := range []int{5, 0, 0} {
			u.turnListPage(-1)
			if u.fit.offset != want {
				t.Fatalf("previous page offset %d, want %d", u.fit.offset, want)
			}
		}
		if u.section != section || u.page != page || u.title != "Current list" || u.playID != 23 || u.index != 1 || u.current.ID != "playing" {
			t.Fatal("page turn changed section or playback")
		}
		u.busy = true
		u.turnListPage(1)
		if u.fit.offset != 0 {
			t.Fatal("page changed during loading")
		}
	}
}
