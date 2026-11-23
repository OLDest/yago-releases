package main

import (
	"strings"
	"yago/internal/music"
)

// Keep the compact page focused on shared choices. Exact native controls remain
// available inline under Advanced, without allocating another window or list.
func (u *UI) visibleWaveGroup(group string, options []music.WaveOption) bool {
	if strings.HasPrefix(group, "shared/") || !hasSharedGroups(options) {
		return true
	}
	if !u.disc.waveAdvanced {
		return false
	}
	for _, pair := range []struct{ native, shared string }{{"yandex/language", "shared/language"}, {"zvuk/language", "shared/language"}, {"yandex/moodEnergy", "shared/mood"}} {
		if group != pair.native {
			continue
		}
		for _, o := range options {
			if o.Group == pair.shared {
				return false
			}
		}
	}
	return true
}
