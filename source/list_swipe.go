package main

import (
	"gioui.org/f32"
	gioevent "gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"math"
	"runtime"
)

type librarySection uint8

const (
	sectionWave librarySection = iota
	sectionLikes
	sectionPlaylists
	sectionOffline
	sectionExplore
)

// One touch record; no animation buffers, timers or cached pages.
type listSwipe struct {
	start                     f32.Point
	id                        pointer.ID
	active, grabbed, rejected bool
}

func (s *listSwipe) update(gtx layout.Context, enabled bool) int {
	direction := 0
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: s, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
		if !ok {
			break
		}
		e := ev.(pointer.Event)
		if !enabled || e.Source != pointer.Touch {
			s.active = false
			continue
		}
		if e.Kind == pointer.Press {
			if s.active {
				s.rejected = true
				continue
			}
			s.start, s.id, s.active, s.grabbed, s.rejected = e.Position, e.PointerID, true, false, false
			continue
		}
		if !s.active || e.PointerID != s.id {
			continue
		}
		dx, dy := e.Position.X-s.start.X, e.Position.Y-s.start.Y
		ax, ay := float32(math.Abs(float64(dx))), float32(math.Abs(float64(dy)))
		switch e.Kind {
		case pointer.Drag:
			if !s.grabbed && ay > float32(gtx.Dp(12)) && ay > ax {
				s.rejected = true
			}
			if !s.rejected && !s.grabbed && ax >= float32(gtx.Dp(24)) && ax > ay*1.5 {
				gtx.Execute(pointer.GrabCmd{Tag: s, ID: s.id})
				s.grabbed = true
			}
		case pointer.Release:
			if s.grabbed && !s.rejected && ax >= float32(gtx.Dp(64)) && ax > ay*1.5 {
				if dx < 0 {
					direction = 1
				} else {
					direction = -1
				}
			}
			s.active = false
		case pointer.Cancel:
			s.active = false
		}
	}
	if !enabled {
		s.active = false
	}
	return direction
}

func (u *UI) swipeEnabled() bool {
	if runtime.GOOS != "android" && windowMode != "phone" {
		return false
	}
	if u.disc.menu != "" || u.qualityMenuOpen || u.up.show || u.code != "" || u.off.picking || u.off.removing != "" {
		return false
	}
	switch u.page {
	case "tracks", "playlists", "offline", "offline-tracks", "discovery":
		return true
	}
	return false
}

func (u *UI) swipeContent(gtx layout.Context, content layout.Widget) layout.Dimensions {
	d := content(gtx)
	if u.swipeEnabled() {
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		pass := pointer.PassOp{}.Push(gtx.Ops)
		gioevent.Op(gtx.Ops, &u.listSwipe)
		pass.Pop()
	}
	return d
}
