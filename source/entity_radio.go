package main

import (
	"strings"
	"yago/internal/music"
)

func (u *UI) startEntityRadio(x menuEntry) {
	if u.api == nil || x.ID == "" {
		return
	}
	y, z := u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds
	ny, nz := cloneWaveSeeds(y), cloneWaveSeeds(z)
	delete(nz, "context")
	if music.IsZvuk(x.ID) {
		id := strings.TrimPrefix(string(x.ID), "zvuk-")
		kind := map[string]string{"track": "TRACK", "artist": "ARTIST", "album": "RELEASE", "playlist": "PLAYLIST"}[x.Group]
		if kind == "" {
			return
		}
		nz["context"] = "zvuk:radio:" + kind + ":" + id
		if entitySeed(ny["context"]) {
			delete(ny, "context")
		}
	} else {
		ny["context"] = x.Group + ":" + string(x.ID)
	}
	u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds = ny, nz
	if e := u.saveServicePreferences(u.disc.prefs); e != nil {
		u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds = y, z
		u.status = e.Error()
		return
	}
	u.feedback("skip")
	u.cancelWaveRefresh()
	u.cancelWaveContinuation()
	u.closeMenu()
	u.section = sectionWave
	u.loadWave(true)
	u.page = "tracks"
	u.viewWave = true
	u.status = "Загружаем волну: " + x.Name
}
func entitySeed(s string) bool {
	for _, prefix := range []string{"track:", "artist:", "album:", "playlist:"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
