package mobile

import (
	"testing"
	"time"
)

func TestPublicationAvoidsIdleIPCAndInterpolatedPositionUpdates(t *testing.T) {
	now := time.Now()
	p := Playback{TrackID: "one", Active: true, Playing: true, Position: 1000}
	s := publicationState{last: p, at: now, sent: true}
	for sec := int64(1); sec < 15; sec++ {
		next := p
		next.Position += sec * 1000
		if s.due(next, now.Add(time.Duration(sec)*time.Second)) {
			t.Fatal("unnecessary position publication", sec)
		}
	}
	next := p
	next.Position += 15000
	if !s.due(next, now.Add(15*time.Second)) {
		t.Fatal("missing periodic position correction")
	}
	p.Playing = false
	s = publicationState{last: p, at: now, sent: true}
	if s.due(p, now.Add(24*time.Hour)) {
		t.Fatal("paused player still publishes")
	}
	p.Active = false
	s = publicationState{last: p, at: now, sent: true}
	if s.due(p, now.Add(24*time.Hour)) {
		t.Fatal("stopped player still publishes")
	}
}

func TestPublicationImmediatelySendsControlsAndSeeks(t *testing.T) {
	now := time.Now()
	p := Playback{TrackID: "one", Active: true, Playing: true, Position: 1000}
	s := publicationState{last: p, at: now, sent: true}
	for _, change := range []func(*Playback){
		func(p *Playback) { p.TrackID = "two" }, func(p *Playback) { p.Playing = false },
		func(p *Playback) { p.Loading = true }, func(p *Playback) { p.Liked = true },
		func(p *Playback) { p.Active = false }, func(p *Playback) { p.Position = 50000 },
		func(p *Playback) { p.Position = 500 },
	} {
		next := p
		change(&next)
		if !s.due(next, now.Add(time.Second)) {
			t.Fatal("state/seek update was throttled", next)
		}
	}
	// A backwards seek can still be beyond the last published position.
	next := p
	next.Position = 5000
	if !s.due(next, now.Add(10*time.Second)) {
		t.Fatal("relative backwards seek missed")
	}
}
