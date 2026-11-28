package main

import (
	"testing"
	"yago/internal/music"
)

func TestPlayingTrackPageFollowsChangesButAllowsBrowsing(t *testing.T) {
	for _, page := range []string{"tracks", "offline-tracks", "discovery"} {
		t.Run(page, func(t *testing.T) {
			tracks := []music.Track{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}, {ID: "f"}}
			u := &UI{page: page, tracks: tracks, current: tracks[4]}
			u.disc.result.Tracks = tracks
			u.followPlayingPage(2, 6)
			if u.fit.offset != 4 {
				t.Fatal("playing track not visible", u.fit.offset)
			}
			u.fit.offset = 0
			u.followPlayingPage(2, 6)
			if u.fit.offset != 0 {
				t.Fatal("manual browsing was overridden")
			}
			u.current = tracks[3] // Also covers shuffled/backward transitions.
			u.followPlayingPage(2, 6)
			if u.fit.offset != 2 {
				t.Fatal("track transition not followed")
			}
			u.followPlayingPage(3, 6)
			if u.fit.offset != 3 {
				t.Fatal("resize hid playing track")
			}
			u.current.ID = "another-list"
			u.followPlayingPage(3, 6)
			if u.fit.offset != 3 {
				t.Fatal("unrelated playback changed page")
			}
		})
	}
}
