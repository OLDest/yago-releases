package main

import (
	"errors"
	"testing"
)

func TestLikeToggleStep(t *testing.T) {
	u, p, track := feedbackFixture(t)
	u.likeTrack(track)
	feedbackDone(t, u)
	if !u.trackLiked(track) {
		t.Fatal("first click failed")
	}
	u.likeTrack(track)
	feedbackDone(t, u)
	if u.trackLiked(track) || len(p.calls) != 2 || p.calls[0] != "like" || p.calls[1] != "unlike" {
		t.Fatal("second click did not unlike", p.calls)
	}
	if u.current.ID != "playing" || u.playID != 17 || u.streamVersion != 9 {
		t.Fatal("playback changed")
	}
}
func TestLikeToggleFailureAndLikesViewStep(t *testing.T) {
	u, p, track := feedbackFixture(t)
	u.disc.view.Kind = "likes"
	p.unlikeErr = errors.New("unavailable")
	u.likeTrack(track)
	e := feedbackDone(t, u)
	if e.err == nil || !u.trackLiked(track) || len(p.calls) != 1 || p.calls[0] != "unlike" {
		t.Fatal("failed unlike corrupted state", p.calls)
	}
}
