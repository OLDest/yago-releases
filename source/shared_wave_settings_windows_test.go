package main

import (
	"strings"
	"testing"
	"yago/internal/music"
	"yago/internal/store"
)

func sharedOptionsFixture(t *testing.T, u *UI) []music.WaveOption {
	t.Helper()
	options := []music.WaveOption{}
	for _, c := range sharedWaveChoices {
		options = append(options, music.WaveOption{Name: c.Name, Group: "yandex/" + c.YGroup, Seed: c.YSeed})
	}
	z, e := u.services.zvuk.WaveOptions(u.ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, o := range z {
		o.Group = "zvuk/" + o.Group
		options = append(options, o)
	}
	return options
}
func TestSharedWaveChoicesPersistBothAccountsAndInferAfterReload(t *testing.T) {
	u := serviceTestUI(t)
	u.services.mode = "all"
	u.applyServiceMode(false)
	u.disc.prefs.Seeds = map[string]string{"context": "activity:run"}
	u.disc.prefs.ZvukSeeds = map[string]string{"popular": "zvuk:popular:1"}
	options := sharedOptionsFixture(t, u)
	u.optionsMenu(options)
	if !hasSharedGroups(u.disc.options) {
		t.Fatal("missing shared groups")
	}
	for _, group := range []string{"yandex/language", "zvuk/language", "yandex/moodEnergy", "zvuk/fun", "zvuk/energy"} {
		found := false
		for _, o := range u.disc.options {
			found = found || o.Group == group
		}
		if !found {
			t.Fatal("provider control hidden", group)
		}
	}
	for _, c := range sharedWaveChoices {
		if c.Group == "shared/genre" {
			continue
		} // Multi-selection is tested separately.
		u.menuAction(menuEntry{Action: "wave-option", Group: c.Group, Seed: c.Seed})
		if u.selectedSharedWaveSeed(c.Group) != c.Seed {
			t.Fatal("shared choice does not round-trip", c.Seed)
		}
		y, e := store.LoadPreferences(stateDir, u.services.yandexAccount.UID)
		if e != nil || y.Seeds[c.YGroup] != c.YSeed {
			t.Fatal("Yandex not saved", e)
		}
		z, e := store.LoadPreferences(stateDir, u.services.zvukAccount.UID)
		if e != nil || z.ZvukSeeds[c.ZGroup] != c.ZSeed || c.ZGroup2 != "" && z.ZvukSeeds[c.ZGroup2] != c.ZSeed2 {
			t.Fatal("Zvuk not saved", e)
		}
		if y.Seeds["context"] != "activity:run" || z.ZvukSeeds["popular"] != "zvuk:popular:1" {
			t.Fatal("unrelated group changed")
		}
		p, e := u.loadServicePreferences(u.account.UID)
		if e != nil {
			t.Fatal(e)
		}
		u.disc.prefs = p
		if u.selectedSharedWaveSeed(c.Group) != c.Seed {
			t.Fatal("lost after reload")
		}
	}
	for _, s := range u.waveSeeds() {
		if strings.HasPrefix(s, "shared:") {
			t.Fatal("UI seed sent to API")
		}
	}
	u.waveGroupMenu("shared/mood")
	n := 0
	for _, c := range u.disc.waveChoices {
		if c.Selected {
			n++
		}
	}
	if n != 1 {
		t.Fatal("not single choice")
	}
	u.menuAction(menuEntry{Action: "wave-reset-all"})
	if len(u.disc.prefs.Seeds) != 0 || len(u.disc.prefs.ZvukSeeds) != 0 {
		t.Fatal("shared reset failed")
	}
}
func TestSharedWaveDoesNotOverwriteConflictingPrefsOnOpen(t *testing.T) {
	u := serviceTestUI(t)
	u.services.mode = "all"
	u.applyServiceMode(false)
	u.disc.prefs.Seeds = map[string]string{"language": "settingLanguage:russian"}
	u.disc.prefs.ZvukSeeds = map[string]string{"language": "zvuk:language:all"}
	u.optionsMenu(sharedOptionsFixture(t, u))
	if u.selectedSharedWaveSeed("shared/language") != "" || u.disc.prefs.Seeds["language"] != "settingLanguage:russian" || u.disc.prefs.ZvukSeeds["language"] != "zvuk:language:all" {
		t.Fatal("conflict silently overwritten")
	}
	u.waveGroupMenu("shared/language")
	if u.disc.menu != "wave-options" || len(u.disc.waveChoices) != 4 {
		t.Fatal("not inline or unsupported values exposed")
	}
	u.menuAction(menuEntry{Action: "wave-option", Group: "shared/language", Seed: "shared:mood:sad"})
	if u.disc.prefs.ZvukSeeds["language"] != "zvuk:language:all" {
		t.Fatal("forged seed accepted")
	}
	u.services.mode = "zvuk"
	z, _ := u.services.zvuk.WaveOptions(u.ctx)
	u.optionsMenu(z)
	if hasSharedGroups(u.disc.options) {
		t.Fatal("single mode changed")
	}
}

func TestSharedSettingsTailPreservesPlayingTrackAndIgnoresStaleResults(t *testing.T) {
	u := serviceTestUI(t)
	u.services.mode = "all"
	u.applyServiceMode(false)
	u.isWave = true
	u.page = "tracks"
	u.viewWave = true
	u.waveNextID = 5
	u.current = music.Track{ID: "playing", Title: "Current"}
	u.queue = []music.Track{{ID: "old"}, u.current, {ID: "old-next"}}
	u.index = 1
	u.order = []int{0, 1, 2}
	u.orderPos = 1
	u.session = "original-session"
	u.batch = "original-batch"
	u.playID = 77
	u.streamVersion = 33
	tracks := waveTracksForTest(200, 15)
	contexts := waveContexts(15, "new-session", "new-batch")
	u.waveContinuationReady(event{kind: "wave-settings-tail", waveID: 4, tracks: tracks, waveContexts: contexts})
	if len(u.queue) != 3 {
		t.Fatal("stale settings replaced queue")
	}
	u.waveContinuationReady(event{kind: "wave-settings-tail", waveID: 5, tracks: tracks, waveContexts: contexts})
	if u.playID != 77 || u.streamVersion != 33 || u.current.ID != "playing" || u.index != 0 || len(u.queue) != 16 || u.waveContexts[0].Session != "original-session" || u.waveContexts[1].Session != "new-session" {
		t.Fatal("current playback or routing changed")
	}
}
