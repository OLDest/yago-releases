package main

import (
	"runtime"
	"time"
)

func (u *UI) backgroundInterval() time.Duration {
	if runtime.GOOS != "android" {
		return time.Second
	}
	s := u.player.Snapshot()
	if s.Playing || s.Loading || u.up.busy || u.qualitySwitching {
		return time.Second
	}
	if u.off.manager != nil && u.off.manager.Snapshot().Active {
		return time.Second
	}
	return 30 * time.Second
}
