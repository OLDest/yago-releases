package main

import (
	"testing"
	"yago/internal/music"
)

func TestInstrumentalLanguagePersistsAndResetsVocal(t *testing.T) {
	u := serviceTestUI(t)
	u.services.mode = "all"
	u.applyServiceMode(false)
	u.disc.prefs.Seeds = map[string]string{"language": "settingLanguage:russian"}
	u.disc.prefs.ZvukSeeds = map[string]string{"language": "zvuk:language:russian", "vocal": "zvuk:vocal:0.7"}
	u.optionsMenu(sharedOptionsFixture(t, u))
	u.menuAction(menuEntry{Action: "wave-option", Group: "shared/language", Seed: "shared:language:instrumental"})
	if u.disc.prefs.Seeds["language"] != "settingLanguage:without-words" || u.disc.prefs.ZvukSeeds["vocal"] != "zvuk:vocal:0" || u.disc.prefs.ZvukSeeds["language"] != "zvuk:language:all" {
		t.Fatal("instrumental mapping")
	}
	p, e := u.loadServicePreferences(u.account.UID)
	if e != nil {
		t.Fatal(e)
	}
	u.disc.prefs = p
	if u.selectedSharedWaveSeed("shared/language") != "shared:language:instrumental" {
		t.Fatal("selection not restored")
	}
	for _, seed := range u.waveSeeds() {
		if seed == music.ExcludeZvukWave {
			t.Fatal("instrumental excluded supported provider")
		}
	}
	u.menuAction(menuEntry{Action: "wave-option", Group: "shared/language", Seed: "shared:language:any"})
	if u.disc.prefs.ZvukSeeds["vocal"] != "zvuk:vocal:1" || u.selectedSharedWaveSeed("shared/language") != "shared:language:any" {
		t.Fatal("vocal remained zero")
	}
	u.menuAction(menuEntry{Action: "wave-option", Group: "shared/language", Seed: "shared:language:foreign"})
	found := false
	for _, seed := range u.waveSeeds() {
		found = found || seed == music.ExcludeZvukWave
	}
	if !found {
		t.Fatal("unsupported language still mixes Zvuk")
	}
}

func TestSharedSettingsHideDuplicatesAndKeepAdvancedInline(t *testing.T) {
	u := serviceTestUI(t)
	u.services.mode = "all"
	u.applyServiceMode(false)
	u.optionsMenu(sharedOptionsFixture(t, u))
	for _, e := range u.disc.entries {
		if e.Action == "wave-group" && len(e.Group) > 0 && e.Group[:min(7, len(e.Group))] != "shared/" {
			t.Fatal("duplicate service group on compact page", e.Group)
		}
	}
	u.menuAction(menuEntry{Action: "wave-advanced"})
	groups := map[string]bool{}
	for _, e := range u.disc.entries {
		if e.Action == "wave-group" {
			groups[e.Group] = true
		}
	}
	if !groups["zvuk/vocal"] || !groups["zvuk/energy"] || !groups["zvuk/fun"] {
		t.Fatal("exact controls lost")
	}
	if groups["yandex/language"] || groups["zvuk/language"] || groups["yandex/moodEnergy"] {
		t.Fatal("identical controls still duplicated")
	}
	if u.disc.menu != "wave-options" {
		t.Fatal("advanced navigated away")
	}
}
