package main

import (
	"context"
	"testing"
	"time"
	"yago/internal/music"
)

func TestCompletedUpdateCancelsItsContext(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &UI{ctx: parent}
	ctx, id := u.updateJob("check")
	called := false
	u.finishUpdate(id, func() { called = true })
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("completed job context retained")
	}
	if !called || u.up.busy || u.up.stop != nil {
		t.Fatal("job completion state")
	}
	next, nextID := u.updateJob("next")
	u.finishUpdate(id, func() { t.Fatal("stale completion ran") })
	select {
	case <-next.Done():
		t.Fatal("stale completion cancelled new job")
	default:
	}
	u.finishUpdate(nextID, func() {})
}
func TestClockTextFormatting(t *testing.T) {
	for _, tt := range []struct {
		seconds float64
		want    string
	}{{-1, "0:00"}, {0, "0:00"}, {9.9, "0:09"}, {60, "1:00"}, {3671, "61:11"}} {
		var b [64]byte
		if got := string(appendClockTime(b[:0], tt.seconds)); got != tt.want {
			t.Fatalf("%v: %s", tt.seconds, got)
		}
	}
}
func TestLibraryReleaseDropsHiddenMetadata(t *testing.T) {
	u := &UI{tracks: []music.Track{{ID: "a"}}, queue: []music.Track{{ID: "a"}}, order: []int{0}, qualities: []music.Quality{music.High}, streamBitrate: 320}
	u.releaseLibraryView()
	if len(u.tracks) != 0 || len(u.playlists) != 0 || len(u.rows) != 0 || len(u.order) != 0 || len(u.qualities) != 0 || u.streamBitrate != 0 {
		t.Fatal("hidden metadata retained")
	}
	if len(u.queue) != 1 {
		t.Fatal("helper must not discard active queue")
	}
}
