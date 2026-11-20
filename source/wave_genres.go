package main

import (
	"strings"
	"yago/internal/music"
)

func containsSeed(raw, seed string) bool {
	for _, s := range music.GenreSeeds(raw) {
		if s == seed {
			return true
		}
	}
	return false
}
func toggleGenre(raw, seed string) string {
	seeds := music.GenreSeeds(raw)
	if seed == "" || seed == "user:onyourwave" || strings.HasSuffix(seed, ":any") {
		return ""
	}
	var out []string
	found := false
	for _, s := range seeds {
		if s == seed {
			found = true
		} else {
			out = append(out, s)
		}
	}
	if !found && len(out) < music.MaxWaveGenres {
		out = append(out, seed)
	}
	return strings.Join(out, "|")
}
func (u *UI) genreSelected(group, seed string) bool {
	if group == "shared/genre" {
		for _, c := range sharedWaveChoices {
			if c.Group == group && c.Seed == seed {
				y, z := u.disc.prefs.Seeds["genre"], u.disc.prefs.ZvukSeeds["genre"]
				if strings.HasSuffix(seed, ":any") {
					return len(music.GenreSeeds(y)) == 0 && len(music.GenreSeeds(z)) == 0
				}
				return containsSeed(y, c.YSeed) && containsSeed(z, c.ZSeed)
			}
		}
		return false
	}
	raw := u.waveSettingsForGroup(group)["genre"]
	if seed == "" || seed == "user:onyourwave" || strings.HasSuffix(seed, ":any") {
		return len(music.GenreSeeds(raw)) == 0
	}
	return containsSeed(raw, seed)
}
func (u *UI) toggleWaveGenre(x menuEntry) {
	if !hasWaveOption(u.disc.options, x.Group, x.Seed) && x.Seed != "" {
		return
	}
	y, z := u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds
	ny, nz := cloneWaveSeeds(y), cloneWaveSeeds(z)
	if x.Group == "shared/genre" {
		for _, c := range sharedWaveChoices {
			if c.Group == x.Group && c.Seed == x.Seed {
				// A partially shared choice selects for both; a full choice deselects both.
				if !u.genreSelected(x.Group, x.Seed) && !strings.HasSuffix(x.Seed, ":any") {
					if !containsSeed(ny["genre"], c.YSeed) {
						ny["genre"] = toggleGenre(ny["genre"], c.YSeed)
					}
					if !containsSeed(nz["genre"], c.ZSeed) {
						nz["genre"] = toggleGenre(nz["genre"], c.ZSeed)
					}
				} else {
					ny["genre"] = toggleGenre(ny["genre"], c.YSeed)
					nz["genre"] = toggleGenre(nz["genre"], c.ZSeed)
				}
				break
			}
		}
	} else if u.waveGroupZvuk(x.Group) {
		nz["genre"] = toggleGenre(nz["genre"], x.Seed)
	} else {
		ny["genre"] = toggleGenre(ny["genre"], x.Seed)
	}
	if ny["genre"] == "" {
		delete(ny, "genre")
	}
	if nz["genre"] == "" {
		delete(nz, "genre")
	}
	u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds = ny, nz
	if e := u.saveServicePreferences(u.disc.prefs); e != nil {
		u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds = y, z
		u.status = e.Error()
		return
	}
	page := u.disc.wavePopupPage
	u.optionsMenu(u.disc.options)
	u.waveGroupMenu(x.Group)
	u.disc.wavePopupPage = page
	u.status = "Можно выбрать несколько жанров. Затем нажмите «Применить»."
}
