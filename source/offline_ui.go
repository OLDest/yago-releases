package main

import (
	"context"
	"fmt"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"path/filepath"
	"strings"
	"yago/internal/music"
	"yago/internal/offline"
)

type offlineUI struct {
	library                                                                     *offline.Library
	manager                                                                     *offline.Manager
	open, destination, saveTrack, saveList, cancel, create, close, remove, keep widget.Clickable
	editor                                                                      widget.Editor
	list                                                                        widget.List
	rows, actions                                                               []widget.Clickable
	playlists                                                                   []offline.Summary
	target, view                                                                string
	picking                                                                     bool
	removing                                                                    music.ID
	revision                                                                    uint64
	error                                                                       string
}

func (u *UI) initOffline() {
	s := &u.off
	s.target = offline.DefaultID
	s.list.Axis = layout.Vertical
	s.editor.SingleLine = true
	s.editor.Submit = true
	library, e := offline.Open(filepath.Join(stateDir, "offline"))
	if e != nil {
		s.error = e.Error()
		return
	}
	s.library = library
	s.manager = offline.NewManager(u.ctx, library, func() {
		if u.w != nil {
			u.w.Invalidate()
		}
	})
	u.refreshOffline()
}
func (u *UI) refreshOffline() {
	s := &u.off
	if s.library == nil {
		return
	}
	s.playlists = s.library.Summaries()
	if len(s.rows) != len(s.playlists) {
		s.rows = make([]widget.Clickable, len(s.playlists))
	}
	if u.page == "offline-tracks" {
		position := u.list.Position
		u.setTracks(s.library.Tracks(s.view))
		u.list.Position = position
	}
}
func (u *UI) pollOffline() {
	if u.off.manager == nil {
		return
	}
	p := u.off.manager.Snapshot()
	if p.Revision != u.off.revision {
		u.off.revision = p.Revision
		u.refreshOffline()
	}
}
func (u *UI) offlineName(id string) string {
	for _, p := range u.off.playlists {
		if p.ID == id {
			return p.Name
		}
	}
	return "Оффлайн"
}
func (u *UI) openOffline() {
	u.section = sectionOffline
	u.viewWave = false
	u.cancelLibrary()
	u.busy = false
	u.up.show = false
	u.off.picking = false
	u.off.removing = ""
	u.page = "offline"
	u.title = "Оффлайн-плейлисты"
	u.refreshOffline()
}
func (u *UI) createOffline() {
	if u.off.library == nil {
		return
	}
	id, e := u.off.library.Create(u.off.editor.Text())
	if e != nil {
		u.status = e.Error()
		return
	}
	u.off.editor.SetText("")
	u.off.target = id
	u.off.picking = false
	u.refreshOffline()
	u.status = "Плейлист создан: " + u.offlineName(id)
}
func (u *UI) saveOffline(tracks []music.Track) {

	s := &u.off
	if s.manager == nil {
		u.status = "Оффлайн-хранилище недоступно: " + s.error
		return
	}
	if len(tracks) == 0 {
		u.status = "Выберите трек или список"
		return
	}
	if len(tracks) > offline.MaxPlaylistTracks {
		u.status = "В одном плейлисте — до 2000 треков. Разделите список."
		return
	}
	if u.api == nil && (!demo || demoURL == "") {
		for _, t := range tracks {
			if _, _, e := s.library.Resolve(t.ID); e != nil {
				u.status = "Для скачивания новых треков войдите в музыкальный сервис"
				return
			}
		}
	}
	jobs := make([]offline.Job, 0, len(tracks))
	for _, t := range tracks {
		job := offline.Job{Track: t, Playlist: s.target, Quality: u.quality}
		if demo && demoURL != "" {
			raw := demoURL
			job.Resolve = func(_ context.Context, _ music.Track, _ music.Quality) (music.StreamInfo, error) {
				return music.StreamInfo{URL: raw}, nil
			}
		} else if u.api != nil {
			api := u.api
			job.Resolve = api.Stream
		}
		jobs = append(jobs, job)
	}
	n, e := s.manager.Enqueue(jobs)
	if e != nil {
		u.status = e.Error()
		return
	}
	u.status = fmt.Sprintf("В очередь загрузки: %d · %s", n, u.offlineName(s.target))
}
func (u *UI) offlineInputs(gtx layout.Context) {
	u.pollOffline()
	s := &u.off
	for s.open.Clicked(gtx) {
		u.openOffline()
	}
	for s.destination.Clicked(gtx) {
		s.picking = true
		u.up.show = false
		u.refreshOffline()
	}
	for s.close.Clicked(gtx) {
		s.picking = false
		s.removing = ""
	}
	for s.saveTrack.Clicked(gtx) {
		if u.current.ID != "" {
			u.saveOffline([]music.Track{u.current})
		} else {
			u.status = "Сначала выберите трек"
		}
	}
	for s.saveList.Clicked(gtx) {
		u.saveOffline(u.tracks)
	}
	for s.cancel.Clicked(gtx) {
		if s.manager != nil {
			s.manager.Cancel()
			u.status = "Оставшиеся загрузки отменены"
		}
	}
	for s.create.Clicked(gtx) {
		u.createOffline()
	}
	for {
		e, ok := s.editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok = e.(widget.SubmitEvent); ok {
			u.createOffline()
		}
	}
	for i := range s.rows {
		if s.rows[i].Clicked(gtx) {
			p := s.playlists[i]
			s.target = p.ID
			if s.picking {
				s.picking = false
				u.status = "Сохраняем в: " + p.Name
			} else {
				s.view = p.ID
				u.page = "offline-tracks"
				u.title = p.Name
				u.setTracks(s.library.Tracks(p.ID))
				u.status = "Локальные файлы · интернет не нужен"
			}
		}
	}
	for i := range s.actions {
		if s.actions[i].Clicked(gtx) && i < len(u.tracks) {
			if u.page == "offline-tracks" {
				s.removing = u.tracks[i].ID
			} else {
				u.saveOffline([]music.Track{u.tracks[i]})
			}
			break
		}
	}
	for s.keep.Clicked(gtx) {
		s.removing = ""
	}
	for s.remove.Clicked(gtx) {
		id := s.removing
		if id == "" || s.library == nil {
			continue
		}
		if e := s.library.Remove(s.view, id); e != nil {
			u.status = e.Error()
			continue
		}
		s.removing = ""
		u.removeFromOfflineQueue(id)
		u.refreshOffline()
		u.status = "Трек убран из плейлиста"
	}
}
func (u *UI) removeFromOfflineQueue(id music.ID) {
	if !u.queueOffline {
		return
	}
	_, _, missing := u.off.library.Resolve(id)
	if u.offlineQueuePlaylist != u.off.view && missing == nil {
		return
	}
	oldIndex := u.index
	currentRemoved := u.current.ID == id
	queue := make([]music.Track, 0, len(u.queue))
	for _, t := range u.queue {
		if t.ID != id {
			queue = append(queue, t)
		}
	}
	u.queue = queue
	if len(queue) == 0 {
		if u.player != nil {
			u.player.Stop()
		}
		u.current = music.Track{}
		u.index = -1
		u.stopped = true
		u.queueOffline = false
		u.offlineQueuePlaylist = ""
		u.serviceActive = false
		u.clearPlayback()
		return
	}
	index := min(max(0, oldIndex), len(queue)-1)
	if !currentRemoved {
		for i, t := range queue {
			if t.ID == u.current.ID {
				index = i
				break
			}
		}
	}
	u.index = index
	u.resetOrder(index)
	if currentRemoved {
		paused := u.pauseRequested || u.stopped
		if u.player != nil {
			paused = paused || !u.player.Snapshot().Playing
		}
		u.startAt(index, 0, paused, false)
	} else {
		u.savePlayback(true)
	}
}
func sizeLabel(bytes int64) string {
	if bytes < 1<<20 {
		return fmt.Sprintf("%.0f КиБ", float64(bytes)/1024)
	}
	if bytes < 1<<30 {
		return fmt.Sprintf("%.1f МиБ", float64(bytes)/(1<<20))
	}
	return fmt.Sprintf("%.2f ГиБ", float64(bytes)/(1<<30))
}
func shortName(s string) string {
	r := []rune(s)
	if len(r) > 18 {
		return string(r[:17]) + "…"
	}
	return s
}
func (u *UI) offlineToolbar(gtx layout.Context) layout.Dimensions {
	s := &u.off
	if s.library == nil {
		return layout.Dimensions{}
	}
	var progress offline.Progress
	if s.manager != nil {
		progress = s.manager.Snapshot()
	}
	showSave := !s.picking && (u.current.ID != "" || len(u.tracks) > 0) && u.page != "offline"
	if !showSave && !progress.Active && progress.Pending == 0 && progress.Done == 0 && progress.Failed == 0 {
		return layout.Dimensions{}
	}
	return layout.Inset{Left: 12, Right: 12, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{}
		if showSave {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return u.phoneButton(gtx, &s.destination, "В: "+shortName(u.offlineName(s.target)), false)
					}), gap(6),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.phoneButton(gtx, &s.saveTrack, "Трек ↓", false)
					}), gap(6),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if len(u.tracks) == 0 {
							return layout.Dimensions{}
						}
						return u.phoneButton(gtx, &s.saveList, "Список ↓", false)
					}))
			}))
		}
		if progress.Active || progress.Pending > 0 || progress.Done > 0 || progress.Failed > 0 {
			children = append(children, gap(4), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				message := fmt.Sprintf("Сохранено: %d", progress.Done)
				amount := ""
				if progress.Active {
					amount = sizeLabel(progress.Downloaded)
					if progress.Total > 0 {
						amount = fmt.Sprintf("%d%%", progress.Downloaded*100/progress.Total)
					}
				} else if progress.Failed > 0 {
					message = fmt.Sprintf("Сохранено: %d · ошибок: %d · %s", progress.Done, progress.Failed, progress.Error)
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					// Keep the filename separate from changing percentages, so the
					// text shaper does not cache every title/percentage combination.
					if progress.Active {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Label(u.textTheme("↓ "+progress.Title), 11, "↓ "+progress.Title)
							l.MaxLines = 1
							l.Color = muted
							return l.Layout(gtx)
						}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.label(gtx, amount, 11, muted) }), gap(8), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return u.label(gtx, fmt.Sprintf("ещё %d", progress.Pending), 11, muted)
							}))
						}))
					}
					l := material.Label(u.textTheme(message), 11, message)
					l.MaxLines = 2
					l.Color = muted
					return l.Layout(gtx)
				}), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !progress.Active && progress.Pending == 0 {
						return layout.Dimensions{}
					}
					return u.phoneButton(gtx, &s.cancel, "Отмена", false)
				}))
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}
func (u *UI) offlineContent(gtx layout.Context) layout.Dimensions {
	s := &u.off
	if s.library == nil {
		return u.label(gtx, "Оффлайн-хранилище недоступно: "+s.error, 14, muted)
	}
	if s.removing != "" {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Убрать трек из этого плейлиста?", 18, white)
		}), gap(12), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Файл удалится, если его больше нет в других плейлистах.", 13, muted)
		}), gap(12), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &s.remove, "Убрать", true) }), gap(8), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &s.keep, "Отмена", false) }))
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			message := "Откройте плейлист. Треки сохраняются кнопкой ↓."
			if s.picking {
				message = "Куда сохранить музыку?"
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return u.label(gtx, message, 14, muted) }), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !s.picking {
					return layout.Dimensions{}
				}
				return u.phoneButton(gtx, &s.close, "Назад", false)
			}))
		}), gap(10),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return widget.Border{Color: muted, Width: 1, CornerRadius: 8}.Layout(gtx, inset(8, func(gtx layout.Context) layout.Dimensions {
					e := material.Editor(u.textTheme(s.editor.Text()), &s.editor, "Новый плейлист")
					e.TextSize = 14
					return e.Layout(gtx)
				}))
			}), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.phoneButton(gtx, &s.create, "Создать", true)
			}))
		}), gap(10),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.fitRows(gtx, "offline-playlists", len(s.playlists), 48, func(gtx layout.Context, i int) layout.Dimensions {
				p := s.playlists[i]
				label := fmt.Sprintf("%s · %d · %s", p.Name, p.Count, sizeLabel(p.Bytes))
				if p.ID == s.target {
					label = "[x] " + label
				}
				return layout.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return u.phoneButton(gtx, &s.rows[i], label, p.ID == s.target)
				})
			})
		}))
}
func (u *UI) offlineRowAction(gtx layout.Context, i int) layout.Dimensions {
	if u.page == "playlists" || i >= len(u.off.actions) {
		return layout.Dimensions{}
	}
	label := "↓"
	if u.page == "offline-tracks" {
		label = "×"
	}
	return layout.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = min(gtx.Constraints.Max.X, max(gtx.Constraints.Min.X, gtx.Dp(40)))
		return u.phoneButton(gtx, &u.off.actions[i], label, false)
	})
}
func (u *UI) isOfflineView() bool { return strings.HasPrefix(u.page, "offline") || u.off.picking }
