package main

import (
	"context"
	"errors"
	"fmt"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"sort"
	"strings"
	"yago/internal/music"
	"yago/internal/store"
)

type exploreView struct {
	Kind, Query, Type, Title string
	ID                       music.ID
	Page                     int
	History                  []music.HistoryRef
	More                     bool
}
type exploreEvent struct {
	View    exploreView
	Result  music.SearchPage
	Options []music.WaveOption
	Owner   music.ID
}
type menuEntry struct {
	Selected          bool
	Label, Action     string
	ID                music.ID
	Name, Group, Seed string
}
type discoveryUI struct {
	waveAdvanced                    bool
	waveApply                       widget.Clickable
	waveOpenGroup                   string
	waveChoices                     []menuEntry
	waveChoiceClicks                []widget.Clickable
	waveDismiss, wavePrev, waveNext widget.Clickable
	wavePopupPage                   int
	settingsClick, blocksClick      widget.Clickable
	dismiss                         widget.Clickable
	view                            exploreView
	result                          music.SearchPage
	prefs                           store.Preferences
	menu                            string
	entries                         []menuEntry
	clicks                          []widget.Clickable
	close, history, actions         widget.Clickable
	blockPage                       int
	options                         []music.WaveOption
	pending                         bool
	pendingID                       uint64
}

func (u *UI) initPreferences() {
	u.disc.pendingID++
	u.disc.pending = false
	u.trackFeedback.liked = nil
	owner := u.account.UID

	p, e := u.loadServicePreferences(owner)
	u.disc.prefs = p
	if e != nil {
		u.status = "Не удалось прочитать блокировки: " + e.Error()
	}
}
func (u *UI) blocked(t music.Track) bool { return u.disc.prefs.Blocked(t) }
func (u *UI) filterRecommendations(tracks []music.Track) []music.Track {
	out := make([]music.Track, 0, len(tracks))
	for _, t := range tracks {
		if !u.blocked(t) {
			out = append(out, t)
		}
	}
	return out
}
func (u *UI) waveSeeds() []string {
	if u.waveIsZvuk() {
		out := []string{"zvuk:personal"}
		for _, v := range u.disc.prefs.ZvukSeeds {
			out = append(out, strings.Split(v, "|")...)
		}
		return out
	}
	seeds := []string{"user:onyourwave"}
	if s := u.disc.prefs.Seeds["context"]; s != "" {
		seeds[0] = s
	}
	for _, s := range music.GenreSeeds(u.disc.prefs.Seeds["genre"]) {
		if seeds[0] == "user:onyourwave" {
			seeds[0] = s
		} else {
			seeds = append(seeds, s)
		}
	}
	for _, group := range []string{"diversity", "moodEnergy", "language", "mood", "energy"} {
		if s := u.disc.prefs.Seeds[group]; s != "" {
			seeds = append(seeds, s)
		}
	}
	if u.waveMixed() {
		if u.disc.prefs.Seeds["language"] == "settingLanguage:not-russian" {
			seeds = append(seeds, music.ExcludeZvukWave)
		}
		seeds = append(seeds, "zvuk:personal")
		for _, v := range u.disc.prefs.ZvukSeeds {
			seeds = append(seeds, strings.Split(v, "|")...)
		}
	}
	return seeds
}
func (u *UI) closeExplore() {
	u.disc.view = exploreView{}
	u.disc.result = music.SearchPage{}
	u.closeMenu()
}
func (u *UI) closeMenu() {
	u.closeWaveDropdown()
	u.fit.menuPage = 0
	u.disc.menu = ""
	u.disc.entries = nil
	u.disc.clicks = nil
	u.disc.options = nil
}
func (u *UI) showMenu(kind string, entries []menuEntry) {
	u.closeWaveDropdown()
	u.fit.menuPage = 0
	u.disc.menu = kind
	u.disc.entries = entries
	u.disc.clicks = make([]widget.Clickable, len(entries))
	u.up.show = false
}
func (u *UI) loadExplore(v exploreView) {

	if u.api == nil {
		u.status = "Сначала войдите в аккаунт"
		return
	}
	if v.Kind == "search" && strings.TrimSpace(v.Query) == "" {
		u.status = "Введите запрос"
		return
	}
	ctx, cancel, g := u.begin(v.Title)
	u.disc.view = v
	api := u.api
	owner := u.account.UID
	var localRefs []music.HistoryRef
	if v.Kind == "history" {
		localRefs = u.readLocalHistory()
	}
	go func() {
		defer cancel()
		e := event{kind: "explore", gen: g}
		x := &exploreEvent{View: v}
		e.explore = x
		switch v.Kind {
		case "likes":
			x.Result.Tracks, x.View.More, e.err = api.LikesPage(ctx, owner, v.Page)
			if music.IsPartialLibrary(e.err) {
				x.Result.Warning = e.err.Error()
				e.err = nil
			}
		case "search":
			x.Result, e.err = api.SearchPage(ctx, v.Query, v.Type, v.Page)
			x.View.More = x.Result.More
		case "artist":
			x.Result.Tracks, x.View.More, e.err = api.ArtistTracks(ctx, v.ID, v.Page)
		case "album":
			x.Result.Tracks, x.View.More, e.err = api.AlbumPage(ctx, v.ID, v.Page)
		case "history":
			if len(v.History) == 0 {
				x.View.History, e.err = api.History(ctx)
				if music.IsPartialLibrary(e.err) {
					x.Result.Warning = e.err.Error()
					e.err = nil
				}
				if e.err != nil && len(localRefs) > 0 {
					x.Result.Warning = e.err.Error()
					e.err = nil
				}
				x.View.History = music.MergeHistory(x.View.History, localRefs, 400)
			}
			if e.err == nil {
				x.Result.Tracks, x.View.More, e.err = api.HistoryTracks(ctx, x.View.History, v.Page)
			}
		}
		u.emit(e)
	}()
}
func (u *UI) searchExplore() {
	u.loadExplore(exploreView{Kind: "search", Query: strings.TrimSpace(u.editor.Text()), Type: "all", Title: "Результаты поиска"})
}
func (u *UI) exploreReady(e event) {
	if e.gen != u.gen || e.explore == nil {
		return
	}
	u.busy = false
	if e.err != nil {
		u.status = e.err.Error()
		return
	}
	x := e.explore
	u.setTracks(x.Result.Tracks)
	u.disc.view = x.View
	if len(x.Result.Albums)+len(x.Result.Artists)+len(x.Result.Playlists) > 0 {
		u.disc.result = x.Result
		u.page = "discovery"
		u.rows = make([]widget.Clickable, u.exploreCount())
	} else {
		u.page = "tracks"
	}
	u.status = fmt.Sprintf("Страница %d · элементов %d", x.View.Page+1, u.exploreCount())
	if u.exploreCount() == 0 {
		u.status = "Ничего не найдено"
	}
	if x.Result.Warning != "" {
		u.status += " · " + x.Result.Warning
	}
}
func (u *UI) exploreCount() int {
	r := u.disc.result
	if u.page == "discovery" {
		return len(r.Tracks) + len(r.Albums) + len(r.Artists) + len(r.Playlists)
	}
	return len(u.tracks)
}
func (u *UI) exploreRow(gtx layout.Context, index int) layout.Dimensions {
	r := &u.disc.result
	i := index
	title, sub := "", ""
	if i < len(r.Tracks) {
		title = r.Tracks[i].Title
		sub = "Трек · " + r.Tracks[i].Artist() + u.sourceSuffix(string(r.Tracks[i].ID))
	} else if i -= len(r.Tracks); i < len(r.Albums) {
		title = r.Albums[i].Title
		sub = "Альбом" + u.sourceSuffix(string(r.Albums[i].ID))
	} else if i -= len(r.Albums); i < len(r.Artists) {
		title = r.Artists[i].Name
		sub = "Исполнитель" + u.sourceSuffix(string(r.Artists[i].ID))
	} else {
		i -= len(r.Artists)
		title = r.Playlists[i].Title
		sub = fmt.Sprintf("Плейлист · %d треков", r.Playlists[i].TrackCount)
		sub += u.sourceSuffix(string(r.Playlists[i].Kind))
	}
	return fixedHeight(64, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.rows[index].Layout(gtx, inset(8, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Label(u.textTheme(title), 14, title)
					l.MaxLines = 1
					return l.Layout(gtx)
				}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Label(u.textTheme(sub), 11, sub)
					l.MaxLines = 1
					l.Color = muted
					return l.Layout(gtx)
				}))
			}))
		}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if index < len(r.Tracks) {
				return u.trackRowActions(gtx, index)
			}
			return layout.Dimensions{}
		}))
	})(gtx)
}
func (u *UI) openExploreItem(i int) {
	r := u.disc.result
	if i < len(r.Tracks) {
		u.feedback("skip")
		u.queue = append([]music.Track(nil), r.Tracks...)
		u.queueOffline = false
		u.offlineQueuePlaylist = ""
		u.isWave = false
		u.resetOrder(i)
		u.start(i)
		return
	}
	i -= len(r.Tracks)
	if i < len(r.Albums) {
		a := r.Albums[i]
		u.loadExplore(exploreView{Kind: "album", ID: a.ID, Title: a.Title})
		return
	}
	i -= len(r.Albums)
	if i < len(r.Artists) {
		a := r.Artists[i]
		u.loadExplore(exploreView{Kind: "artist", ID: a.ID, Title: a.Name})
		return
	}
	i -= len(r.Artists)
	if i >= 0 && i < len(r.Playlists) {
		u.loadPlaylist(r.Playlists[i])
	}
}
func (u *UI) discoveryToolbar(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(600))
	gtx.Constraints.Min.X = min(gtx.Constraints.Min.X, gtx.Constraints.Max.X)
	return layout.Inset{Left: 12, Right: 12, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !u.hasWaveService() {
				return layout.Dimensions{}
			}
			return u.phoneButton(gtx, &u.disc.settingsClick, "Настройки", false)
		}), gap(6), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.phoneButton(gtx, &u.disc.history, "История", false)
		}), gap(6), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.phoneButton(gtx, &u.disc.blocksClick, "Блокировки", false)
		}))
	})
}
func (u *UI) pagedContent(gtx layout.Context) layout.Dimensions  { return u.content(gtx) }
func (u *UI) discoveryMenu(gtx layout.Context) layout.Dimensions { return u.menuGrid(gtx) }
func (u *UI) actionsMenu() {
	t := u.current
	if t.ID == "" {
		u.status = "Выберите трек"
		return
	}
	entries := []menuEntry{{Label: "Снять лайк", Action: "unlike", ID: t.ID}, {Label: "Не рекомендовать этот трек", Action: "dislike", ID: t.ID}}
	entries = append(entries, menuEntry{Label: "Волна по треку", Action: "entity-radio", Group: "track", ID: t.ID, Name: t.Title})
	for _, a := range t.Artists {
		if a.ID != "" {
			entries = append(entries, menuEntry{Label: "Волна: " + a.Name, Action: "entity-radio", Group: "artist", ID: a.ID, Name: a.Name})
		}
	}
	for _, a := range t.Albums[:min(1, len(t.Albums))] {
		entries = append(entries, menuEntry{Label: "Волна по альбому", Action: "entity-radio", Group: "album", ID: a.ID, Name: a.Title})
	}
	if u.disc.view.Kind == "playlist" && music.IsZvuk(u.disc.view.ID) {
		entries = append(entries, menuEntry{Label: "Волна по открытому плейлисту", Action: "entity-radio", Group: "playlist", ID: u.disc.view.ID, Name: u.disc.view.Title})
	}
	if u.disc.prefs.Tracks[t.ID] {
		entries[1].Label = "Разрешить этот трек"
		entries[1].Action = "allow-track"
	}
	for _, a := range t.Artists {
		if a.ID == "" {
			continue
		}
		action, label := "block-artist", "Заблокировать: "+a.Name
		if _, ok := u.disc.prefs.Artists[a.ID]; ok {
			action, label = "allow-artist", "Разблокировать: "+a.Name
		}
		entries = append(entries, menuEntry{Label: label, Action: action, ID: a.ID, Name: a.Name})
	}
	if u.fit.compact {
		entries = append(entries, menuEntry{Label: "Куда сохранять оффлайн", Action: "offline-target"})
	}
	u.showMenu("actions", entries)
}
func blockedTrackDisplayName(track music.Track, fallback string) string {
	name := strings.TrimSpace(track.Title)
	if name == "" {
		name = strings.TrimSpace(fallback)
	}
	if name == "" {
		return ""
	}
	if artist := strings.TrimSpace(track.Artist()); artist != "" {
		name = artist + " — " + name
	}
	if len(name) > 512 {
		cutoff := 0
		for i := range name {
			if i > 512 {
				break
			}
			cutoff = i
		}
		name = name[:cutoff]
	}
	return name
}

func (u *UI) blockedMenu(page int) {
	var entries []menuEntry
	for id, n := range u.disc.prefs.Artists {
		entries = append(entries, menuEntry{Label: "Разблокировать: " + n, Action: "allow-artist", ID: id, Name: n})
	}
	for id := range u.disc.prefs.Tracks {
		name := strings.TrimSpace(u.disc.prefs.TrackNames[id])
		if name == "" {
			for _, tracks := range [][]music.Track{u.tracks, u.queue, {u.current}} {
				for _, track := range tracks {
					if track.ID == id && strings.TrimSpace(track.Title) != "" {
						name = blockedTrackDisplayName(track, "")
						break
					}
				}
				if name != "" {
					break
				}
			}
		}
		if name == "" {
			name = string(id)
		}
		entries = append(entries, menuEntry{Label: "Разрешить трек " + name, Action: "allow-track", ID: id, Name: name})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Label < entries[j].Label })
	start := page * music.PageSize
	if start >= len(entries) {
		start = 0
		page = 0
	}
	end := min(start+music.PageSize, len(entries))
	out := append([]menuEntry(nil), entries[start:end]...)
	if start > 0 {
		out = append(out, menuEntry{Label: "< Предыдущие", Action: "blocks-prev"})
	}
	if end < len(entries) {
		out = append(out, menuEntry{Label: "Следующие >", Action: "blocks-next"})
	}
	if len(out) == 0 {
		out = append(out, menuEntry{Label: "Заблокированных нет"})
	}
	u.disc.blockPage = page
	u.showMenu("blocks", out)
}
func (u *UI) loadWaveOptions() {

	if u.api == nil {
		u.status = "Сначала войдите в аккаунт"
		return
	}
	ctx, cancel, g := u.beginOverlay()
	api := u.waveAPI()
	u.showMenu("loading", []menuEntry{{Label: "Загрузка настроек…"}})
	go func() {
		defer cancel()
		options, e := api.WaveOptions(ctx)
		u.emit(event{kind: "wave-options", gen: g, err: e, explore: &exploreEvent{Options: options}})
	}()
}
func (u *UI) optionsMenu(options []music.WaveOption) {
	if u.waveMixed() && !hasSharedGroups(options) {
		options = sharedOptions(options)
	}
	entries := []menuEntry{}
	seen := map[string]bool{}
	for _, o := range options {
		if u.waveMixed() && !u.visibleWaveGroup(o.Group, options) {
			continue
		}
		if seen[o.Group] {
			continue
		}
		seen[o.Group] = true
		value := "По умолчанию"
		if strings.HasPrefix(o.Group, "shared/") {
			value = "Разные настройки"
		}
		selected := u.selectedWaveSeed(options, o.Group)
		for _, choice := range options {
			if choice.Group == o.Group && choice.Seed == selected {
				value = waveChoiceLabel(choice.Name)
				break
			}
		}
		if waveRawGroup(o.Group) == "genre" {
			var labels []string
			for _, c := range options {
				if c.Group == o.Group && u.genreSelected(c.Group, c.Seed) {
					labels = append(labels, c.Name)
				}
			}
			if len(labels) > 0 {
				value = strings.Join(labels, ", ")
			} else {
				value = "Разные настройки"
			}
		}
		entries = append(entries, menuEntry{Label: u.waveGroupLabel(o.Group) + "\n" + value + "  v", Action: "wave-group", Group: o.Group})
	}
	if u.waveMixed() {
		label := "Дополнительно"
		if u.disc.waveAdvanced {
			label = "Скрыть дополнительные настройки"
		}
		entries = append(entries, menuEntry{Label: label, Action: "wave-advanced"})
		entries = append(entries, menuEntry{Label: "Сбросить настройки всех сервисов", Action: "wave-reset-all"})
	} else {
		entries = append(entries, menuEntry{Label: "Сбросить настройки этого сервиса", Action: "wave-reset"})
	}
	u.showMenu("wave-options", entries)
	u.disc.options = options
}
func (u *UI) waveGroupMenu(group string) {
	if u.disc.waveOpenGroup == group {
		u.closeWaveDropdown()
		return
	}
	entries := []menuEntry{}
	selected := u.selectedWaveSeed(u.disc.options, group)
	if selected == "" && !strings.HasPrefix(group, "shared/") {
		entries = append(entries, menuEntry{Label: "По умолчанию", Action: "wave-option", Group: group, Selected: true})
	}
	for _, o := range u.disc.options {
		if o.Group == group {
			chosen := o.Seed == selected
			label := waveChoiceLabel(o.Name)
			if waveRawGroup(group) == "genre" {
				chosen = u.genreSelected(group, o.Seed)
				if chosen {
					label = "[x] " + label
				} else {
					label = "[ ] " + label
				}
			}
			entries = append(entries, menuEntry{Label: label, Selected: chosen, Action: "wave-option", Seed: o.Seed, Group: group})
		}
	}
	u.disc.waveOpenGroup = group
	u.disc.waveChoices = entries
	u.disc.waveChoiceClicks = make([]widget.Clickable, len(entries))
	u.disc.wavePopupPage = 0
}
func (u *UI) discoveryInputs(gtx layout.Context) {
	if u.disc.waveApply.Clicked(gtx) {
		u.applyWaveSettings()
		return
	}
	if u.disc.waveDismiss.Clicked(gtx) {
		u.closeWaveDropdown()
	}
	if u.disc.wavePrev.Clicked(gtx) {
		u.disc.wavePopupPage = max(0, u.disc.wavePopupPage-1)
	}
	if u.disc.waveNext.Clicked(gtx) {
		u.disc.wavePopupPage++
	}
	for i := range u.disc.waveChoiceClicks {
		if u.disc.waveChoiceClicks[i].Clicked(gtx) {
			x := u.disc.waveChoices[i]
			u.menuAction(x)
			break
		}
	}
	for u.disc.actions.Clicked(gtx) {
		if u.disc.menu == "actions" {
			u.closeMenu()
		} else {
			u.qualityMenuOpen = false
			u.actionsMenu()
		}
	}
	for u.disc.dismiss.Clicked(gtx) {
		u.closeMenu()
	}
	for u.disc.close.Clicked(gtx) {
		if u.code != "" {
			if u.loginCancel != nil {
				u.loginCancel()
				u.loginCancel = nil
			}
			u.code = ""
			u.authBusy = false
			u.gen++
		}
		if u.disc.menu == "loading" {
			if u.libraryCancel != nil {
				u.libraryCancel()
				u.libraryCancel = nil
				u.gen++
			}
			u.busy = false
		}
		u.closeMenu()
	}
	for u.disc.history.Clicked(gtx) {
		u.loadExplore(exploreView{Kind: "history", Title: "История прослушивания"})
	}
	for u.disc.settingsClick.Clicked(gtx) {
		u.loadWaveOptions()
	}
	for u.disc.blocksClick.Clicked(gtx) {
		u.blockedMenu(0)
	}
	for i := range u.disc.clicks {
		if u.disc.clicks[i].Clicked(gtx) {
			entry := u.disc.entries[i]
			u.menuAction(entry)
			break
		}
	}
}
func (u *UI) menuAction(x menuEntry) {
	if u.serviceAction(x.Action) {
		return
	}
	switch x.Action {
	case "wave-advanced":
		u.disc.waveAdvanced = !u.disc.waveAdvanced
		u.optionsMenu(u.disc.options)
	case "entity-radio":
		u.startEntityRadio(x)
	case "offline-target":
		u.closeMenu()
		u.off.picking = true
		u.refreshOffline()
	case "blocks":
		u.blockedMenu(0)
	case "blocks-next":
		u.blockedMenu(u.disc.blockPage + 1)
	case "blocks-prev":
		u.blockedMenu(max(0, u.disc.blockPage-1))
	case "wave-options":
		u.loadWaveOptions()
	case "wave-group":
		u.waveGroupMenu(x.Group)
	case "wave-groups":
		u.optionsMenu(u.disc.options)
	case "wave-option", "wave-reset":
		if x.Action == "wave-option" && waveRawGroup(x.Group) == "genre" {
			u.toggleWaveGenre(x)
			return
		}
		if strings.HasPrefix(x.Group, "shared/") {
			u.applySharedWaveChoice(x)
			return
		}
		old := u.waveSettingsForGroup(x.Group)
		next := make(map[string]string, len(old))
		for k, v := range old {
			next[k] = v
		}
		if x.Action == "wave-reset" {
			next = nil
		} else {
			if x.Seed == "" {
				delete(next, waveRawGroup(x.Group))
			} else {
				next[waveRawGroup(x.Group)] = x.Seed
			}
		}
		u.setWaveSettingsForGroup(x.Group, next)
		if e := u.saveServicePreferences(u.disc.prefs); e != nil {
			u.setWaveSettingsForGroup(x.Group, old)
			u.status = e.Error()
			return
		}
		options := u.disc.options
		u.optionsMenu(options)
		u.cancelWaveRefresh()
		u.status = "Настройки сохранены. Нажмите «Применить», чтобы запустить новую волну."
	case "wave-reset-all":
		y, z := u.disc.prefs.Seeds, u.disc.prefs.ZvukSeeds
		u.disc.prefs.Seeds = nil
		u.disc.prefs.ZvukSeeds = nil
		if e := u.saveServicePreferences(u.disc.prefs); e != nil {
			u.disc.prefs.Seeds = y
			u.disc.prefs.ZvukSeeds = z
			u.status = e.Error()
			return
		}
		u.optionsMenu(u.disc.options)
		u.cancelWaveRefresh()
		u.status = "Настройки обоих сервисов сброшены. Нажмите «Применить»."
	case "unlike", "dislike", "allow-track", "block-artist", "allow-artist":
		u.moderate(x)
	}
}
func (u *UI) beginOverlay() (context.Context, context.CancelFunc, uint64) {
	if u.libraryCancel != nil {
		u.libraryCancel()
	}
	u.gen++
	u.busy = false
	ctx, cancel := context.WithCancel(u.ctx)
	u.libraryCancel = cancel
	return ctx, cancel, u.gen
}
func (u *UI) featureEvent(e event) bool {
	switch e.kind {
	case "explore":
		u.exploreReady(e)
		return true
	case "wave-options":
		if e.gen != u.gen {
			return true
		}
		u.busy = false
		if e.err != nil {
			u.closeMenu()
			u.status = e.err.Error()
			return true
		}
		if e.explore != nil {
			u.optionsMenu(e.explore.Options)
		}
		return true
	case "moderated":
		if e.featureID != u.disc.pendingID || e.explore == nil || e.explore.Owner != u.account.UID {
			return true
		}
		u.disc.pending = false
		if e.feedbackTrack != "" && e.feedbackLike != nil {
			u.markTrackLiked(e.feedbackTrack, *e.feedbackLike)
		}
		u.status = e.message
		if e.err != nil {
			u.status += " · " + e.err.Error()
		}
		return true
	}
	return false
}

// Keep network jobs scoped to the signed-in account without long-lived polling.
func (u *UI) moderate(x menuEntry) {
	t := music.Track{ID: x.ID}
	for _, tracks := range [][]music.Track{u.tracks, u.queue, {u.current}} {
		for _, candidate := range tracks {
			if candidate.ID == x.ID {
				t = candidate
				break
			}
		}
	}
	u.moderateTrack(x, t)
}

func (u *UI) moderateTrack(x menuEntry, t music.Track) {

	if u.disc.pending {
		u.status = "Дождитесь предыдущего отклика"
		return
	}
	if u.api == nil {
		u.status = "Для отклика войдите в аккаунт"
		return
	}
	remove := strings.HasPrefix(x.Action, "allow-")
	kind := "track"
	if strings.HasSuffix(x.Action, "artist") {
		kind = "artist"
	}
	if x.Action != "unlike" && x.Action != "like" {
		p := u.disc.prefs
		if kind == "artist" {
			next := make(map[music.ID]string, len(p.Artists)+1)
			for k, v := range p.Artists {
				next[k] = v
			}
			if remove {
				delete(next, x.ID)
			} else {
				next[x.ID] = x.Name
			}
			p.Artists = next
		} else {
			next := make(map[music.ID]bool, len(p.Tracks)+1)
			for k, v := range p.Tracks {
				next[k] = v
			}
			if remove {
				delete(next, x.ID)
			} else {
				next[x.ID] = true
			}
			p.Tracks = next
			names := make(map[music.ID]string, len(p.TrackNames)+1)
			for id, name := range p.TrackNames {
				names[id] = name
			}
			if remove {
				delete(names, x.ID)
			} else if name := blockedTrackDisplayName(t, x.Name); name != "" {
				names[x.ID] = name
			}
			p.TrackNames = names
		}
		if e := u.saveServicePreferences(p); e != nil {
			u.status = "Не удалось сохранить блокировку: " + e.Error()
			return
		}
		u.disc.prefs = p
	}
	u.closeMenu()
	if x.Action != "unlike" && x.Action != "like" && !remove {
		if kind == "track" {
			u.markTrackLiked(x.ID, false)
		}
		u.cancelWaveRefresh()
		u.waveTracks = u.filterRecommendations(u.waveTracks)
		if u.viewWave {
			u.setTracks(u.filterRecommendations(u.tracks))
		}
		if u.blocked(u.current) {
			u.advance(false)
			if u.blocked(u.current) {
				u.player.Stop()
				u.stopped = true
				u.serviceActive = false
				u.advancePending = true
				u.savePlayback(true)
			}
		}
	}

	api, uid, ctx := u.api, u.account.UID, u.ctx
	u.disc.pending = true
	u.disc.pendingID++
	id := u.disc.pendingID
	u.status = "Отправляем отклик в " + music.Source(x.ID) + "…"
	go func() {
		var e error
		var liked *bool
		message := "Блокировка сохранена в YaGo и сервисе"
		if x.Action == "like" {
			e = api.Like(ctx, uid, t)
			message = "Добавлено в «Мне нравится»"
			if e == nil {
				v := true
				liked = &v
			} else {
				message = "Не удалось поставить ♥"
			}
		} else if x.Action == "unlike" {
			e = api.Unlike(ctx, uid, t)
			message = "Отметка ♥ снята"
			if e != nil {
				message = "Не удалось снять ♥"
			} else {
				v := false
				liked = &v
			}
		} else {
			var unlikeErr error
			if kind == "track" && !remove {
				unlikeErr = api.Unlike(ctx, uid, t)
			}
			e = api.Dislike(ctx, uid, kind, x.ID, remove)
			if remove {
				message = "Блокировка снята в YaGo"
			}
			if e != nil && !remove {
				message = "Блокировка действует в YaGo; сервис не принял отклик"
			}
			if unlikeErr != nil {
				message += "; не удалось снять ♥ в сервисе"
				e = errors.Join(e, unlikeErr)
			}
		}
		u.emit(event{kind: "moderated", featureID: id, message: message, err: e, feedbackTrack: t.ID, feedbackLike: liked, explore: &exploreEvent{Owner: uid}})
	}()
}
