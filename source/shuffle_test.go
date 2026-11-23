package main

import (
	"testing"
	"yago/internal/music"
)

func TestPlaybackOrder(t *testing.T) {
	for _, n := range []int{0, 1, 2, 40} {
		for current := 0; current < max(1, n); current++ {
			for _, shuffle := range []bool{false, true} {
				order := playbackOrder(n, current, shuffle)
				if len(order) != n {
					t.Fatal("wrong length")
				}
				seen := map[int]bool{}
				for p, i := range order {
					if i < 0 || i >= n || seen[i] {
						t.Fatal("invalid or repeated track")
					}
					seen[i] = true
					if !shuffle && p != i {
						t.Fatal("natural order lost")
					}
				}
				if shuffle && n > 0 && order[0] != current {
					t.Fatal("current track moved")
				}
			}
		}
	}
}
func TestShuffleNavigation(t *testing.T) {
	u := &UI{shuffled: true, index: 5}
	u.queue = makeTracksForShuffle(12)
	u.resetOrder(5)
	seen := map[int]bool{5: true}
	for pos := 1; pos < 12; pos++ {
		i := u.nextIndex()
		if i < 0 || seen[i] {
			t.Fatal("repeat before queue ends")
		}
		seen[i] = true
		u.index = i
		u.orderPos = pos
		if u.previousIndex() != u.order[pos-1] {
			t.Fatal("back does not follow playback history")
		}
	}
	if u.nextIndex() != -1 {
		t.Fatal("queue must end")
	}
	current := u.index
	u.shuffled = false
	u.resetOrder(current)
	if u.orderPos != current {
		t.Fatal("turning off must preserve current track")
	}
}

func makeTracksForShuffle(n int) []music.Track { return make([]music.Track, n) }
