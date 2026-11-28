package main

import (
	"context"
	"errors"
	"image"
	"sync"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"yago/internal/music"
	"yago/internal/store"
)

type feedbackProbe struct {
	music.Backend
	mu                    sync.Mutex
	calls                 []string
	track                 music.Track
	unlikeErr, dislikeErr error
}

func (p *feedbackProbe) Like(_ context.Context, _ music.ID, track music.Track) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "like")
	p.track = track
	return nil
}
func (p *feedbackProbe) Unlike(_ context.Context, _ music.ID, track music.Track) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "unlike")
	p.track = track
	return p.unlikeErr
}
func (p *feedbackProbe) Dislike(_ context.Context, _ music.ID, kind string, id music.ID, remove bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, kind+":"+string(id))
	if remove {
		p.calls[len(p.calls)-1] += " allow"
	}
	return p.dislikeErr
}
func feedbackFixture(t *testing.T) (*UI, *feedbackProbe, music.Track) {
	t.Helper()
	old := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = old })
	probe := &feedbackProbe{}
	track := music.Track{ID: "row", Title: "Not playing", Albums: []music.Album{{ID: "album"}}}
	u := &UI{ctx: context.Background(), api: &music.API{Backend: probe}, account: music.Account{UID: "owner"}, events: make(chan event, 4), current: music.Track{ID: "playing"}, playID: 17, streamVersion: 9, index: 0}
	u.disc.prefs = store.Preferences{Owner: "owner"}
	u.queue = []music.Track{u.current, track}
	u.setTracks([]music.Track{track})
	return u, probe, track
}
func feedbackDone(t *testing.T, u *UI) event {
	t.Helper()
	select {
	case e := <-u.events:
		if !u.featureEvent(e) {
			t.Fatal("feedback event ignored")
		}
		return e
	case <-time.After(time.Second):
		t.Fatal("feedback timed out")
		return event{}
	}
}
func TestLikeNonPlayingRowPreservesTrackAndPlayback(t *testing.T) {
	u, p, track := feedbackFixture(t)
	u.likeTrack(track)
	e := feedbackDone(t, u)
	if e.err != nil || !u.trackLiked(track) || p.track.FullID() != "row:album" {
		t.Fatal(e.err, p.track, u.trackFeedback.liked)
	}
	if u.current.ID != "playing" || u.playID != 17 || u.streamVersion != 9 || u.index != 0 || len(u.queue) != 2 {
		t.Fatal("like changed playback")
	}
}
func TestBlockRowRemovesLikeAndPersistsRecommendationExclusion(t *testing.T) {
	for _, failure := range []string{"", "unlike", "dislike"} {
		t.Run(failure, func(t *testing.T) {
			u, p, track := feedbackFixture(t)
			if failure == "unlike" {
				p.unlikeErr = errors.New("unlike unavailable")
			}
			if failure == "dislike" {
				p.dislikeErr = errors.New("dislike unavailable")
			}
			u.markTrackLiked(track.ID, true)
			u.waveTracks = []music.Track{u.current, track}
			u.moderateTrack(menuEntry{Action: "dislike", ID: track.ID}, track)
			e := feedbackDone(t, u)
			if len(p.calls) != 2 || p.calls[0] != "unlike" || p.calls[1] != "track:row" || p.track.FullID() != "row:album" {
				t.Fatal(p.calls, p.track)
			}
			if !u.blocked(track) || u.trackLiked(track) || len(u.waveTracks) != 1 || len(u.filterRecommendations([]music.Track{track, u.current})) != 1 {
				t.Fatal("block/like/filter inconsistent")
			}
			prefs, err := store.LoadPreferences(stateDir, "owner")
			if err != nil || !prefs.Blocked(track) {
				t.Fatal("block not persisted", prefs, err)
			}
			if (e.err != nil) != (failure != "") {
				t.Fatal("incorrect failure report", e.err)
			}
			if u.current.ID != "playing" || u.playID != 17 || u.streamVersion != 9 || len(u.queue) != 2 {
				t.Fatal("row block changed current playback")
			}
		})
	}
}
func TestUnblockDoesNotRestoreLikeAndBlockedTrackCannotBeLiked(t *testing.T) {
	u, p, track := feedbackFixture(t)
	u.disc.prefs.Tracks = map[music.ID]bool{track.ID: true}
	u.markTrackLiked(track.ID, false)
	u.likeTrack(track)
	if u.disc.pending || len(p.calls) != 0 {
		t.Fatal("liked blocked track")
	}
	u.moderateTrack(menuEntry{Action: "allow-track", ID: track.ID}, track)
	feedbackDone(t, u)
	if u.blocked(track) || u.trackLiked(track) || len(p.calls) != 1 || p.calls[0] != "track:row allow" {
		t.Fatal(p.calls)
	}
}
func TestTrackRowActionHitAreasAreSeparateFromPlayback(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		for _, action := range []string{"block", "download", "like"} {
			t.Run(action+map[bool]string{false: "-tracks", true: "-mixed"}[mixed], func(t *testing.T) {
				u := &UI{theme: newUITheme(), page: "tracks"}
				u.setTracks([]music.Track{{ID: "row", Title: "Track"}})
				if mixed {
					u.page = "discovery"
					u.disc.result = music.SearchPage{Tracks: u.tracks}
				}
				var router input.Router
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(320, 64))}
				if mixed {
					u.exploreRow(gtx, 0)
				} else {
					u.trackRow(gtx, 0)
				}
				router.Frame(&ops)
				x := map[string]float32{"block": 200, "download": 246, "like": 286}[action]
				if mixed {
					x += 14
				}
				router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, 30)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, 30)})
				block, download, like := u.trackFeedback.blocks[0].Clicked(gtx), u.off.actions[0].Clicked(gtx), u.trackFeedback.likes[0].Clicked(gtx)
				if block != (action == "block") || download != (action == "download") || like != (action == "like") || u.rows[0].Clicked(gtx) {
					t.Fatal("click leaked or missed", block, download, like)
				}
			})
		}
	}
}
