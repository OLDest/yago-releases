package main

import (
	"strings"
	"testing"
	"yago/internal/music"
)

func TestSharedGenresMultiplePersistAndReset(t *testing.T) {
	u := serviceTestUI(t)
	u.services.mode = "all"
	u.applyServiceMode(false)
	u.optionsMenu(sharedOptionsFixture(t, u))
	for _, s := range []string{"shared:genre:rock", "shared:genre:jazz"} {
		u.menuAction(menuEntry{Action: "wave-option", Group: "shared/genre", Seed: s})
	}
	if !u.genreSelected("shared/genre", "shared:genre:rock") || !u.genreSelected("shared/genre", "shared:genre:jazz") || u.disc.waveOpenGroup != "shared/genre" {
		t.Fatal("multi-select did not stay open")
	}
	p, e := u.loadServicePreferences(u.account.UID)
	if e != nil {
		t.Fatal(e)
	}
	u.disc.prefs = p
	seeds := u.waveSeeds()
	for _, s := range []string{"genre:rock", "genre:jazz", "zvuk:genre:rock", "zvuk:genre:jazz"} {
		found := false
		for _, v := range seeds {
			found = found || s == v
		}
		if !found {
			t.Fatal("missing native genre", s)
		}
	}
	for _, s := range seeds {
		if strings.Contains(s, "|") || strings.HasPrefix(s, "shared:") {
			t.Fatal("UI encoding sent to server")
		}
	}
	u.menuAction(menuEntry{Action: "wave-option", Group: "shared/genre", Seed: "shared:genre:rock"})
	if u.genreSelected("shared/genre", "shared:genre:rock") || !u.genreSelected("shared/genre", "shared:genre:jazz") {
		t.Fatal("toggle changed unrelated genre")
	}
	u.menuAction(menuEntry{Action: "wave-option", Group: "shared/genre", Seed: "shared:genre:any"})
	if len(music.GenreSeeds(u.disc.prefs.Seeds["genre"])) != 0 || len(music.GenreSeeds(u.disc.prefs.ZvukSeeds["genre"])) != 0 {
		t.Fatal("reset lost")
	}
}
