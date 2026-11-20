package main

import (
	"context"
	"strings"
	"time"
	"yago/internal/music"
)

// Shared values are UI-only. Only native seeds reach either service.
type sharedWaveChoice struct {
	Name, Group, Seed, YGroup, YSeed string
	ZGroup, ZSeed, ZGroup2, ZSeed2   string
}

var sharedWaveChoices = []sharedWaveChoice{
	{Name: "Любой", Group: "shared/genre", Seed: "shared:genre:any", YGroup: "genre", YSeed: "user:onyourwave", ZGroup: "genre", ZSeed: "zvuk:genre:any"},
	{Name: "Рок", Group: "shared/genre", Seed: "shared:genre:rock", YGroup: "genre", YSeed: "genre:rock", ZGroup: "genre", ZSeed: "zvuk:genre:rock"},
	{Name: "Метал", Group: "shared/genre", Seed: "shared:genre:metal", YGroup: "genre", YSeed: "genre:metal", ZGroup: "genre", ZSeed: "zvuk:genre:metal"},
	{Name: "Инди", Group: "shared/genre", Seed: "shared:genre:indie", YGroup: "genre", YSeed: "genre:indie", ZGroup: "genre", ZSeed: "zvuk:genre:indie"},
	{Name: "Поп", Group: "shared/genre", Seed: "shared:genre:pop", YGroup: "genre", YSeed: "genre:pop", ZGroup: "genre", ZSeed: "zvuk:genre:pop"},
	{Name: "Электроника", Group: "shared/genre", Seed: "shared:genre:electronic", YGroup: "genre", YSeed: "genre:electronic", ZGroup: "genre", ZSeed: "zvuk:genre:electronic"},
	{Name: "Рэп и хип-хоп", Group: "shared/genre", Seed: "shared:genre:hiphop", YGroup: "genre", YSeed: "genre:rap", ZGroup: "genre", ZSeed: "zvuk:genre:hip_hop"},
	{Name: "Джаз", Group: "shared/genre", Seed: "shared:genre:jazz", YGroup: "genre", YSeed: "genre:jazz", ZGroup: "genre", ZSeed: "zvuk:genre:jazz"},
	{Name: "Классика", Group: "shared/genre", Seed: "shared:genre:classical", YGroup: "genre", YSeed: "genre:classical", ZGroup: "genre", ZSeed: "zvuk:genre:classical"},
	{Name: "Саундтреки", Group: "shared/genre", Seed: "shared:genre:soundtrack", YGroup: "genre", YSeed: "genre:soundtrack", ZGroup: "genre", ZSeed: "zvuk:genre:soundtrack"},
	{Name: "Шансон", Group: "shared/genre", Seed: "shared:genre:chanson", YGroup: "genre", YSeed: "genre:chanson", ZGroup: "genre", ZSeed: "zvuk:genre:chanson"},
	{Name: "R&B", Group: "shared/genre", Seed: "shared:genre:rnb", YGroup: "genre", YSeed: "genre:rnb", ZGroup: "genre", ZSeed: "zvuk:genre:rnb"},
	{Name: "Соул", Group: "shared/genre", Seed: "shared:genre:soul", YGroup: "genre", YSeed: "genre:soul", ZGroup: "genre", ZSeed: "zvuk:genre:soul"},
	{Name: "Эмбиент", Group: "shared/genre", Seed: "shared:genre:ambient", YGroup: "genre", YSeed: "genre:ambient", ZGroup: "genre", ZSeed: "zvuk:genre:ambient"},
	{Name: "Любой", Group: "shared/language", Seed: "shared:language:any", YGroup: "language", YSeed: "settingLanguage:any", ZGroup: "language", ZSeed: "zvuk:language:all", ZGroup2: "vocal", ZSeed2: "zvuk:vocal:1"},
	{Name: "Русский", Group: "shared/language", Seed: "shared:language:russian", YGroup: "language", YSeed: "settingLanguage:russian", ZGroup: "language", ZSeed: "zvuk:language:russian", ZGroup2: "vocal", ZSeed2: "zvuk:vocal:1"},
	{Name: "Без слов", Group: "shared/language", Seed: "shared:language:instrumental", YGroup: "language", YSeed: "settingLanguage:without-words", ZGroup: "language", ZSeed: "zvuk:language:all", ZGroup2: "vocal", ZSeed2: "zvuk:vocal:0"},
	{Name: "Иностранный · только Яндекс", Group: "shared/language", Seed: "shared:language:foreign", YGroup: "language", YSeed: "settingLanguage:not-russian", ZGroup: "language", ZSeed: "zvuk:language:all", ZGroup2: "vocal", ZSeed2: "zvuk:vocal:1"},
	{Name: "Любое", Group: "shared/mood", Seed: "shared:mood:any", YGroup: "moodEnergy", YSeed: "settingMoodEnergy:all", ZGroup: "fun", ZSeed: "zvuk:fun:0.5", ZGroup2: "energy", ZSeed2: "zvuk:energy:0.5"},
	{Name: "Бодрое", Group: "shared/mood", Seed: "shared:mood:active", YGroup: "moodEnergy", YSeed: "settingMoodEnergy:active", ZGroup: "fun", ZSeed: "zvuk:fun:0.5", ZGroup2: "energy", ZSeed2: "zvuk:energy:1"},
	{Name: "Весёлое", Group: "shared/mood", Seed: "shared:mood:fun", YGroup: "moodEnergy", YSeed: "settingMoodEnergy:fun", ZGroup: "fun", ZSeed: "zvuk:fun:1", ZGroup2: "energy", ZSeed2: "zvuk:energy:0.5"},
	{Name: "Спокойное", Group: "shared/mood", Seed: "shared:mood:calm", YGroup: "moodEnergy", YSeed: "settingMoodEnergy:calm", ZGroup: "fun", ZSeed: "zvuk:fun:0.5", ZGroup2: "energy", ZSeed2: "zvuk:energy:0"},
	{Name: "Грустное", Group: "shared/mood", Seed: "shared:mood:sad", YGroup: "moodEnergy", YSeed: "settingMoodEnergy:sad", ZGroup: "fun", ZSeed: "zvuk:fun:0", ZGroup2: "energy", ZSeed2: "zvuk:energy:0.5"},
}

func hasWaveOption(options []music.WaveOption, group, seed string) bool {
	for _, o := range options {
		if o.Group == group && o.Seed == seed {
			return true
		}
	}
	return false
}
func sharedOptions(options []music.WaveOption) []music.WaveOption {
	var out []music.WaveOption
	for _, c := range sharedWaveChoices {
		if !hasWaveOption(options, "yandex/"+c.YGroup, c.YSeed) || !hasWaveOption(options, "zvuk/"+c.ZGroup, c.ZSeed) {
			continue
		}
		if c.ZGroup2 != "" && !hasWaveOption(options, "zvuk/"+c.ZGroup2, c.ZSeed2) {
			continue
		}
		out = append(out, music.WaveOption{Name: c.Name, Group: c.Group, Seed: c.Seed})
	}
	for _, o := range options {
		out = append(out, o)
	}
	return out
}
func effectiveZvukSeed(seeds map[string]string, group string) string {
	if s := seeds[group]; s != "" {
		return s
	}
	switch group {
	case "genre":
		return "zvuk:genre:any"
	case "language":
		return "zvuk:language:all"
	case "fun":
		return "zvuk:fun:0.5"
	case "energy":
		return "zvuk:energy:0.5"
	case "vocal":
		return "zvuk:vocal:1"
	}
	return ""
}
func (u *UI) selectedSharedWaveSeed(group string) string {
	for _, c := range sharedWaveChoices {
		if c.Group != group {
			continue
		}
		y := u.disc.prefs.Seeds[c.YGroup]
		if y == "" {
			if c.YGroup == "genre" {
				y = "user:onyourwave"
			} else if c.YGroup == "language" {
				y = "settingLanguage:any"
			} else {
				y = "settingMoodEnergy:all"
			}
		}
		if y == c.YSeed && effectiveZvukSeed(u.disc.prefs.ZvukSeeds, c.ZGroup) == c.ZSeed && (c.ZGroup2 == "" || effectiveZvukSeed(u.disc.prefs.ZvukSeeds, c.ZGroup2) == c.ZSeed2) {
			return c.Seed
		}
	}
	return ""
}
func cloneWaveSeeds(old map[string]string) map[string]string {
	v := make(map[string]string, len(old)+2)
	for k, s := range old {
		v[k] = s
	}
	return v
}
func (u *UI) applySharedWaveChoice(x menuEntry) {
	if !u.waveMixed() || !hasWaveOption(u.disc.options, x.Group, x.Seed) {
		return
	}
	for _, c := range sharedWaveChoices {
		if c.Group != x.Group || c.Seed != x.Seed {
			continue
		}
		y, z := u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds
		ny, nz := cloneWaveSeeds(y), cloneWaveSeeds(z)
		ny[c.YGroup] = c.YSeed
		nz[c.ZGroup] = c.ZSeed
		if c.ZGroup2 != "" {
			nz[c.ZGroup2] = c.ZSeed2
		}
		u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds = ny, nz
		if e := u.saveServicePreferences(u.disc.prefs); e != nil {
			u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds = y, z
			_ = u.saveServicePreferences(u.disc.prefs)
			u.status = e.Error()
			return
		}
		u.optionsMenu(u.disc.options)
		u.cancelWaveRefresh()
		u.status = "Выбор сохранён для Яндекса и Звука."
		u.reloadSharedWaveTail()
		return
	}
}

// Replace upcoming recommendations only after a fresh batch is ready. The current
// decoder and its original feedback context keep running without a seek/restart.
func (u *UI) reloadSharedWaveTail() {
	if !u.waveMixed() || !u.isWave || u.queueOffline || u.stopped || u.current.ID == "" || u.waveAPI() == nil {
		return
	}
	waiting := u.waveWaiting
	u.cancelWaveContinuation()
	u.waveWaiting = waiting
	ctx, cancel := context.WithTimeout(u.ctx, 45*time.Second)
	u.waveNextCancel = cancel
	u.waveNextID++
	id := u.waveNextID
	api, seeds, prefs := u.waveAPI(), u.waveSeeds(), u.disc.prefs
	ids := make([]string, 0, min(64, len(u.queue)))
	for _, t := range u.queue[max(0, len(u.queue)-64):] {
		ids = append(ids, string(t.ID))
	}
	u.status = "Выбор применён к обоим сервисам. Обновляем следующие треки…"
	go func() {
		defer cancel()
		tracks, contexts, e := fetchWaveTail(ctx, api, "", ids, seeds, prefs)
		u.emit(event{kind: "wave-settings-tail", waveID: id, tracks: tracks, waveContexts: contexts, err: e})
	}()
}
func hasSharedGroups(options []music.WaveOption) bool {
	for _, o := range options {
		if strings.HasPrefix(o.Group, "shared/") {
			return true
		}
	}
	return false
}
