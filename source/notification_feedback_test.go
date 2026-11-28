package main

import (
	"testing"
	"yago/internal/audio"
	"yago/internal/music"
)

func TestNotificationLikeTogglesCurrentTrackAndRejectsStaleTrack(t *testing.T) {
	u, probe, track := feedbackFixture(t)
	u.current = track
	if !u.notificationFeedback("like:previous-track") || u.disc.pending || len(probe.calls) != 0 {
		t.Fatal("stale notification changed another track")
	}
	for _, want := range []bool{true, false} {
		u.mediaCommand("like:" + string(track.ID))
		if e := feedbackDone(t, u); e.err != nil {
			t.Fatal(e.err)
		}
		if u.trackLiked(track) != want {
			t.Fatal("notification like did not toggle")
		}
	}
	if len(probe.calls) != 2 || probe.calls[0] != "like" || probe.calls[1] != "unlike" || u.playID != 17 {
		t.Fatal("notification like changed playback or wrong API calls", probe.calls)
	}
}

func TestNotificationDislikeBlocksCurrentTrackAndStopsExhaustedQueue(t *testing.T) {
	u, probe, track := feedbackFixture(t)
	u.player = audio.New(t.TempDir(), nil)
	defer u.player.Close()
	u.current = track
	u.queue = []music.Track{track}
	u.resetOrder(0)
	u.mediaCommand("dislike:" + string(track.ID))
	if e := feedbackDone(t, u); e.err != nil {
		t.Fatal(e.err)
	}
	if !u.blocked(track) || !u.stopped || len(probe.calls) != 2 || probe.calls[1] != "track:row" {
		t.Fatal("notification dislike failed", probe.calls)
	}
}

func TestNotificationLikeDoesNotInferUnrelatedPlayingTrackIsLiked(t *testing.T) {
	u, _, track := feedbackFixture(t)
	u.disc.view.Kind = "likes"
	if u.trackLiked(u.current) || !u.trackLiked(track) {
		t.Fatal("likes page leaked its state into another current track")
	}
	for _, cmd := range []string{"like", "like:", "dislike:old"} {
		u.notificationFeedback(cmd)
		if u.disc.pending {
			t.Fatal("invalid feedback accepted", cmd)
		}
	}
}
