//go:build windows || android

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"yago/internal/music"
	"yago/internal/musify"
)

func newGuestMusify(dir string) (*music.Musify, error) {
	raw, err := guestMusifyInstallID(dir)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	provider := &music.Musify{Client: musify.New(client, raw)}
	if saved, err := os.ReadFile(filepath.Join(dir, "musify-chart")); err == nil {
		provider.SetChartSource(string(saved))
	}
	return provider, nil
}

func guestMusifyInstallID(dir string) (string, error) {
	path := filepath.Join(dir, "musify-install-id")
	raw, err := os.ReadFile(path)
	if err == nil {
		decoded, e := hex.DecodeString(string(raw))
		if e != nil || len(decoded) != 16 {
			return "", fmt.Errorf("неверный локальный ID установки")
		}
		return string(raw), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	value := hex.EncodeToString(id[:])
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		return "", err
	}
	return value, nil
}

func (u *UI) openMusifyCharts() {
	provider := u.services.musify
	if provider == nil {
		return
	}
	u.section = sectionWave
	ctx, cancel, gen := u.begin("Чарты · Musify")
	go func() {
		defer cancel()
		tracks, err := provider.Charts(ctx)
		u.emit(event{kind: "tracks", gen: gen, tracks: tracks, err: err})
	}()
}

func (u *UI) openChartSettings() {
	u.chartAdvanced = false
	if u.services.musify != nil {
		u.chartDraft = u.services.musify.ChartSource()
	}
	u.showChartSettings()
}
func (u *UI) showChartSettings() {
	provider := u.services.musify
	if provider == nil {
		return
	}
	u.showMenu("musify-chart-settings", []menuEntry{
		{Label: "Дополнительно", Action: "musify-chart-advanced"},
		{Label: "Сводный чарт", Action: "musify-chart-default", Selected: u.chartDraft == ""},
		{Label: "Популярное за неделю", Action: "musify-chart-week", Selected: u.chartDraft == "week"},
		{Label: "Популярное за месяц", Action: "musify-chart-month", Selected: u.chartDraft == "month"},
		{Label: "Жанр", Action: "musify-chart-genres"},
		{Label: "Настроение и занятия", Action: "musify-chart-moods"},
	})
}

func (u *UI) loadMusifyChartGroup(group string) {
	provider := u.services.musify
	if provider == nil {
		return
	}
	draft, advanced := strings.Split(u.chartDraft, ","), u.chartAdvanced
	ctx, cancel, gen := u.beginOverlay()
	u.showMenu("loading", []menuEntry{{Label: "Загружаем настройки Musify…"}})
	go func() {
		defer cancel()
		genres, moods, themes, err := provider.Client.ChartOptions(ctx)
		choices, prefix := genres, "genre:"
		if group == "moods" {
			choices, prefix = moods, "mood:"
		}
		entries := []menuEntry{{Label: "Назад к настройкам чарта", Action: "back-musify-chart"}}
		if advanced {
			entries[0] = menuEntry{Label: "Назад в «Дополнительно»", Action: "musify-chart-advanced"}
		}
		for _, o := range choices {
			if o.ID <= 0 || o.Name == "" {
				continue
			}
			source := prefix + strconv.FormatInt(o.ID, 10)
			entries = append(entries, menuEntry{Label: o.Name, Action: "musify-chart-" + source, Selected: slices.Contains(draft, source)})
		}
		if group == "moods" {
			for _, o := range themes {
				if o.ID <= 0 || o.Name == "" {
					continue
				}
				source := "theme:" + strconv.FormatInt(o.ID, 10)
				entries = append(entries, menuEntry{Label: "Подборки · " + o.Name, Action: "musify-chart-" + source, Selected: slices.Contains(draft, source)})
			}
		}
		u.emit(event{kind: "musify-chart-group", gen: gen, err: err, chartEntries: entries})
	}()
}

func (u *UI) selectMusifyChart(source string) {
	provider := u.services.musify
	if provider == nil {
		return
	}
	if source == "advanced" {
		u.chartAdvanced = true
		u.showAdvancedChartSettings()
		return
	}
	if source == "clear-mix" {
		u.chartDraft = ""
		u.showAdvancedChartSettings()
		return
	}
	if source == "genres" || source == "moods" {
		u.loadMusifyChartGroup(source)
		return
	}
	if source == "default" {
		source = ""
	}
	if !musify.ValidChartSource(source) {
		return
	}
	if u.chartAdvanced && strings.Contains(source, ":") {
		parts := strings.Split(u.chartDraft, ",")
		parts = slices.DeleteFunc(parts, func(part string) bool { return !strings.Contains(part, ":") })
		i := slices.Index(parts, source)
		if i >= 0 {
			parts = slices.Delete(parts, i, i+1)
		} else {
			if len(parts) >= musify.MaxChartMix {
				u.status = "Можно выбрать до шести подборок"
				return
			}
			parts = append(parts, source)
		}
		u.chartDraft = strings.Join(parts, ",")
		for i := range u.disc.entries {
			if u.disc.entries[i].Action == "musify-chart-"+source {
				u.disc.entries[i].Selected = slices.Contains(parts, source)
			}
		}
		u.status = fmt.Sprintf("Выбрано подборок: %d", len(parts))
		return
	}
	u.chartDraft = source
	u.showChartSettings()
}

func (u *UI) applyMusifyChartSettings() {
	provider := u.services.musify
	if provider == nil {
		return
	}
	source := u.chartDraft
	if err := os.WriteFile(filepath.Join(stateDir, "musify-chart"), []byte(source), 0600); err != nil {
		u.status = "Не удалось сохранить чарт Musify"
		return
	}
	provider.SetChartSource(source)
	u.closeMenu()
	if u.services.mode == "musify" {
		u.openMusifyCharts()
	} else {
		u.status = "Настройка Musify применится при пополнении общей волны"
	}
}

func (u *UI) showAdvancedChartSettings() {
	count := 0
	for _, part := range strings.Split(u.chartDraft, ",") {
		if strings.Contains(part, ":") {
			count++
		}
	}
	u.showMenu("musify-chart-settings", []menuEntry{
		{Label: fmt.Sprintf("Жанры и настроение · выбрано %d", count), Action: "musify-chart-genres"},
		{Label: "Настроение, занятия и плейлисты", Action: "musify-chart-moods"},
		{Label: "Очистить выбор", Action: "musify-chart-clear-mix"},
		{Label: "Назад к настройкам", Action: "back-musify-chart"},
	})
}
