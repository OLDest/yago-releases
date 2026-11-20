package main

import (
	"testing"
	"yago/internal/music"
	"yago/internal/store"
)

func TestWaveProviderSettingsIsolationAndRestore(t *testing.T) {
	u := serviceTestUI(t)
	u.services.mode = "all"
	u.applyServiceMode(false)
	u.disc.prefs.Seeds = map[string]string{"language": "language:russian"}
	u.services.waveSource = "zvuk"
	u.setWaveSettingsForGroup("zvuk/energy", map[string]string{"energy": "zvuk:energy:1"})
	if u.waveAPI() != u.api || len(u.waveSeeds()) != 4 {
		t.Fatal("wrong provider/seeds")
	}
	if e := u.saveServicePreferences(u.disc.prefs); e != nil {
		t.Fatal(e)
	}
	y, e := store.LoadPreferences(stateDir, u.services.yandexAccount.UID)
	if e != nil || y.Seeds["language"] != "language:russian" || len(y.ZvukSeeds) != 0 {
		t.Fatal("Yandex settings changed", e)
	}
	z, e := store.LoadPreferences(stateDir, u.services.zvukAccount.UID)
	if e != nil || z.ZvukSeeds["energy"] != "zvuk:energy:1" || len(z.Seeds) != 0 {
		t.Fatal("Zvuk settings lost", e)
	}
	u.restoreWaveSource("123")
	if u.waveAPI() != u.api {
		t.Fatal("All lost mixed API")
	}
	u.restoreWaveSource(music.ID("zvuk-123"))
	if u.waveAPI() != u.api {
		t.Fatal("restored Zvuk replaced mixed API")
	}
	u.waveTracks = []music.Track{{ID: "123"}, {ID: "zvuk-123"}}
	u.chooseWave()
	if !u.viewWave || len(u.tracks) != 2 || u.disc.menu != "" {
		t.Fatal("All wave should open mixed list directly")
	}
}

func TestWaveChoiceReplacesOnlyOneGroupAndResetsOnlyItsService(t *testing.T) {
	u := serviceTestUI(t)
	u.services.mode = "all"
	u.applyServiceMode(false)
	u.services.waveSource = "zvuk"
	u.disc.prefs.Seeds = map[string]string{"language": "language:russian"}
	u.disc.prefs.ZvukSeeds = map[string]string{"energy": "zvuk:energy:0", "language": "zvuk:language:russian"}
	options, e := u.services.zvuk.WaveOptions(u.ctx)
	if e != nil {
		t.Fatal(e)
	}
	for i := range options {
		options[i].Group = "zvuk/" + options[i].Group
	}
	u.optionsMenu(options)
	u.menuAction(menuEntry{Action: "wave-option", Group: "zvuk/energy", Seed: "zvuk:energy:1"})
	if u.disc.prefs.ZvukSeeds["energy"] != "zvuk:energy:1" || u.disc.prefs.ZvukSeeds["language"] != "zvuk:language:russian" || len(u.disc.prefs.ZvukSeeds) != 2 {
		t.Fatal("choice changed another group")
	}
	u.waveGroupMenu("zvuk/energy")
	n := 0
	for _, x := range u.disc.waveChoices {
		if x.Selected {
			n++
			if x.Seed != "zvuk:energy:1" {
				t.Fatal("wrong choice")
			}
		}
	}
	if n != 1 {
		t.Fatal("multiple choices")
	}
	u.menuAction(menuEntry{Action: "wave-reset", Group: "zvuk/energy"})
	if len(u.disc.prefs.ZvukSeeds) != 0 || u.disc.prefs.Seeds["language"] != "language:russian" {
		t.Fatal("reset affected other provider")
	}
}
