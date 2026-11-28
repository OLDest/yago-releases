package main

import (
	"gioui.org/layout"
	"math/rand/v2"
	"os"
	"path/filepath"
)

func playbackOrder(n, current int, shuffle bool) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	if !shuffle || n < 2 || current < 0 || current >= n {
		return order
	}
	order[0], order[current] = order[current], order[0]
	rand.Shuffle(n-1, func(i, j int) { order[i+1], order[j+1] = order[j+1], order[i+1] })
	return order
}
func (u *UI) resetOrder(current int) {
	u.order = playbackOrder(len(u.queue), current, u.shuffled)
	u.orderPos = 0
	for p, i := range u.order {
		if i == current {
			u.orderPos = p
			break
		}
	}
}
func (u *UI) nextIndex() int {
	if len(u.order) != len(u.queue) {
		u.resetOrder(u.index)
	}
	for p := u.orderPos + 1; p < len(u.order); p++ {
		if !u.blocked(u.queue[u.order[p]]) {
			return u.order[p]
		}
	}
	return -1
}
func (u *UI) previousIndex() int {
	if len(u.order) != len(u.queue) {
		u.resetOrder(u.index)
	}
	if len(u.order) == 0 {
		return -1
	}
	for p := max(0, u.orderPos-1); p >= 0; p-- {
		if !u.blocked(u.queue[u.order[p]]) {
			return u.order[p]
		}
	}
	return -1
}
func (u *UI) shuffleButton(gtx layout.Context) layout.Dimensions {
	label := "Шафл"
	if u.shuffled {
		label = "Шафл: вкл"
	}
	return u.button(gtx, &u.shuffle, label, u.shuffled)
}
func (u *UI) toggleShuffle() {
	u.shuffled = !u.shuffled
	u.resetOrder(u.index)
	p := filepath.Join(stateDir, "shuffle")
	if u.shuffled {
		_ = os.WriteFile(p, []byte("1"), 0600)
	} else {
		_ = os.Remove(p)
	}
}
