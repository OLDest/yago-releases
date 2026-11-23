package main

import (
	"context"
	"errors"
	"testing"
	"time"
	"yago/internal/audio"
)

func TestFailedTracksAdvanceInPlaybackOrderAndStop(t *testing.T) {
	withPlaybackDemo(t)
	u := demoPlaybackUI(t)
	u.queue = waveTracksForTest(10, 8)
	u.order = []int{3, 7, 1, 6, 0, 2, 4, 5}
	u.orderPos, u.index, u.playID = 0, 3, 100
	u.isWave = true
	u.current = u.queue[3]
	// The resolver fails before decoding or opening an audio device.
	demoURL = "http://127.0.0.1:1/unavailable"
	for attempt := 0; attempt < 5; attempt++ {
		id := u.playID
		u.advanceIfEnded(audio.Snapshot{ID: id, Error: "HTTP 403"})
		if attempt < 4 && u.index != u.order[attempt+1] {
			t.Fatal("failure did not follow shuffle order", u.index, attempt)
		}
		// Repeated notifications from the failed track must not skip again.
		index := u.index
		u.advanceIfEnded(audio.Snapshot{ID: id, Error: "HTTP 403"})
		if u.index != index {
			t.Fatal("duplicate failure skipped another track")
		}
	}
	if !u.stopped || !u.advancePending || u.failedTracks != 5 || u.waveNextCancel != nil {
		t.Fatal("failure chain was not bounded", u.failedTracks, u.stopped)
	}
}

func TestPlaybackFailureGuardsAndRecovery(t *testing.T) {
	u := &UI{playID: 7, failedTracks: 3, requestedOffset: 40}
	for _, s := range []audio.Snapshot{
		{ID: 0, Error: "failed"}, {ID: 6, Error: "failed"},
		{ID: 7, Paused: true, Error: "failed"},
	} {
		u.advanceIfEnded(s)
	}
	if u.failedTracks != 3 {
		t.Fatal("stale or paused failure handled")
	}
	u.advanceIfEnded(audio.Snapshot{ID: 7, Playing: true, Position: 41})
	if u.failedTracks != 3 {
		t.Fatal("restored offset counted as successful playback")
	}
	u.advanceIfEnded(audio.Snapshot{ID: 7, Playing: true, Position: 46})
	if u.failedTracks != 0 {
		t.Fatal("successful playback did not reset failure count")
	}
	u.advanceIfEnded(audio.Snapshot{ID: 7, Error: "device", DeviceError: true})
	if u.failedTracks != 0 || !u.advancePending {
		t.Fatal("device failure skipped tracks")
	}
}

func TestResolverErrorKeepsPlaybackIdentity(t *testing.T) {
	p := audio.New(t.TempDir(), nil)
	defer p.Close()
	id := p.PlayAt(func(context.Context) (string, error) { return "", errors.New("unavailable") }, 12, true)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s := p.Snapshot()
		if s.Error != "" {
			if s.ID != id || s.Loading || !s.Paused || s.Position != 12 {
				t.Fatal("resolver error discarded playback state", s)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("resolver did not finish")
}
