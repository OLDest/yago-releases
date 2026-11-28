package main

import (
	"gioui.org/layout"
	"gioui.org/widget/material"
	"image/color"
	"strings"
	"yago/internal/music"
)

func waveGroupName(group string) string {
	group = waveRawGroup(group)
	switch group {
	case "genre":
		return "Жанр"
	case "context":
		return "Занятие"
	case "diversity", "popular":
		return "Подбор музыки"
	case "moodEnergy":
		return "Настроение и энергия"
	case "mood", "fun":
		return "Настроение"
	case "energy":
		return "Энергия"
	case "language":
		return "Язык"
	case "vocal":
		return "Вокал"
	}
	return group
}
func waveChoiceLabel(name string) string {
	if _, v, ok := strings.Cut(name, ": "); ok {
		return v
	}
	return name
}
func (u *UI) selectedWaveSeed(options []music.WaveOption, group string) string {
	if strings.HasPrefix(group, "shared/") {
		return u.selectedSharedWaveSeed(group)
	}
	if seed := u.waveSettingsForGroup(group)[waveRawGroup(group)]; seed != "" {
		return seed
	}
	if waveRawGroup(group) == "genre" {
		if u.waveGroupZvuk(group) {
			return "zvuk:genre:any"
		}
		return "user:onyourwave"
	}
	if u.waveGroupZvuk(group) {
		return map[string]string{"popular": "zvuk:popular:0.5", "energy": "zvuk:energy:0.5", "fun": "zvuk:fun:0.5", "language": "zvuk:language:all", "vocal": "zvuk:vocal:1"}[waveRawGroup(group)]
	}
	for _, o := range options {
		if o.Group == group && (o.Seed == "user:onyourwave" || strings.HasSuffix(o.Name, ": Любое") || strings.HasSuffix(o.Name, ": Любой")) {
			return o.Seed
		}
	}
	return ""
}
func (u *UI) waveSourceName() string {
	if u.waveMixed() {
		return "Яндекс + Звук"
	}
	if u.waveIsZvuk() {
		return "Звук"
	}
	return "Яндекс"
}
func (u *UI) waveSourceColor() color.NRGBA {
	if u.waveIsZvuk() {
		return color.NRGBA{R: 181, G: 154, B: 255, A: 255}
	}
	return accent
}
func (u *UI) waveSettingsPanel(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(inset(6, func(gtx layout.Context) layout.Dimensions {
		caption := "Настройки волны · " + u.waveSourceName()
		hint := "Жанры: несколько вариантов. Остальные группы: один вариант"
		if u.waveMixed() {
			hint = "Общие группы применяются к Яндексу и Звуку"
		}
		if u.disc.menu == "musify-chart-settings" {
			caption = "Настройки волны · Musify"
			hint = "Выберите жанр, настроение или период, затем примените настройки"
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(u.textTheme(caption), 18, caption)
			l.Color = u.waveSourceColor()
			l.MaxLines = 1
			return l.Layout(gtx)
		}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Label(u.textTheme(hint), 12, hint)
			l.Color = muted
			l.MaxLines = 2
			return l.Layout(gtx)
		}))
	})), layout.Flexed(1, u.menuGrid), layout.Rigid(inset(6, func(gtx layout.Context) layout.Dimensions {
		return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := "Запустить новую волну"
			if u.disc.menu == "musify-chart-settings" {
				label = "Применить настройки"
			}
			return u.phoneButton(gtx, &u.disc.waveApply, label, true)
		})
	})))
}

func (u *UI) applyWaveSettings() {
	if u.waveAPI() == nil {
		u.status = "Для применения настроек подключите профиль"
		return
	}
	y, z := u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds
	ny, nz := cloneWaveSeeds(y), cloneWaveSeeds(z)
	delete(nz, "context")
	if entitySeed(ny["context"]) {
		delete(ny, "context")
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
}

func waveRawGroup(group string) string {
	if _, raw, ok := strings.Cut(group, "/"); ok {
		return raw
	}
	return group
}
func (u *UI) waveGroupZvuk(group string) bool {
	return strings.HasPrefix(group, "zvuk/") || !strings.Contains(group, "/") && u.waveIsZvuk()
}
func (u *UI) waveSettingsForGroup(group string) map[string]string {
	if u.waveGroupZvuk(group) {
		return u.disc.prefs.ZvukSeeds
	}
	return u.disc.prefs.Seeds
}
func (u *UI) setWaveSettingsForGroup(group string, v map[string]string) {
	if u.waveGroupZvuk(group) {
		u.disc.prefs.ZvukSeeds = v
	} else {
		u.disc.prefs.Seeds = v
	}
}
func (u *UI) waveGroupLabel(group string) string {
	name := waveGroupName(group)
	if strings.HasPrefix(group, "shared/") {
		return "Общее · " + name
	}
	if u.waveMixed() {
		if u.waveGroupZvuk(group) {
			return "Звук · " + name
		}
		return "Яндекс · " + name
	}
	return name
}
func (u *UI) waveGroupColor(group string) color.NRGBA {
	if strings.HasPrefix(group, "shared/") {
		return color.NRGBA{R: 111, G: 218, B: 219, A: 255}
	}
	if u.waveGroupZvuk(group) {
		return color.NRGBA{R: 181, G: 154, B: 255, A: 255}
	}
	return accent
}
func (u *UI) closeWaveDropdown() {
	u.disc.waveOpenGroup = ""
	u.disc.waveChoices = nil
	u.disc.waveChoiceClicks = nil
	u.disc.wavePopupPage = 0
}

func (u *UI) waveSettings() map[string]string {
	if u.waveIsZvuk() {
		return u.disc.prefs.ZvukSeeds
	}
	return u.disc.prefs.Seeds
}
func (u *UI) setWaveSettings(v map[string]string) {
	if u.waveIsZvuk() {
		u.disc.prefs.ZvukSeeds = v
	} else {
		u.disc.prefs.Seeds = v
	}
}

func (u *UI) waveTitle() string {
	if u.waveMixed() {
		return "Общая волна"
	}
	if u.waveIsZvuk() {
		return "Мой поток · Звук"
	}
	return "Моя волна"
}
