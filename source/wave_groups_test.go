package main

import (
	"testing"
	"yago/internal/music"
)

func TestWaveGroupsSingleSelection(t *testing.T) {
	u := &UI{}
	options := []music.WaveOption{{Name: "Моя волна", Group: "context", Seed: "user:onyourwave"}, {Name: "Танцую", Group: "context", Seed: "activity:dance"}, {Name: "По языку: Русский", Group: "language", Seed: "language:russian"}, {Name: "По языку: Любой", Group: "language", Seed: "language:any"}}
	u.optionsMenu(options)
	if len(u.disc.entries) != 3 || u.disc.entries[0].Action != "wave-group" {
		t.Fatal("not grouped", u.disc.entries)
	}
	u.waveGroupMenu("language")
	selected := 0
	for _, e := range u.disc.waveChoices {
		if e.Selected {
			selected++
			if e.Seed != "language:any" {
				t.Fatal("wrong default")
			}
		}
	}
	if selected != 1 {
		t.Fatal("expected one default selection", selected)
	}
	u.closeWaveDropdown()
	u.disc.prefs.Seeds = map[string]string{"context": "activity:dance", "language": "language:russian"}
	u.waveGroupMenu("language")
	selected = 0
	for _, e := range u.disc.waveChoices {
		if e.Selected {
			selected++
			if e.Seed != "language:russian" {
				t.Fatal("wrong saved selection")
			}
		}
	}
	if selected != 1 {
		t.Fatal("multiple selections")
	}
	if u.waveSettings()["context"] != "activity:dance" {
		t.Fatal("other group changed")
	}
}
