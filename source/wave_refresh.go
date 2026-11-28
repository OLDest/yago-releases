package main

import (
	"context"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"yago/internal/music"
)

func (u *UI) canRefreshWave() bool {
	return u.viewWave && u.page == "tracks" && !u.off.picking && u.off.removing == "" && !u.up.show && u.disc.menu == ""
}
func (u *UI) refreshShortcut(gtx layout.Context) {
	for {
		e, ok := gtx.Event(key.Filter{Name: key.NameF5})
		if !ok {
			return
		}
		if k, ok := e.(key.Event); ok && k.State == key.Press && u.canRefreshWave() {
			u.refreshWave()
		}
	}
}

func (u *UI) viewHeading(gtx layout.Context, size unit.Sp) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		l := material.Label(u.textTheme(u.title), size, u.title)
		l.MaxLines = 1
		return l.Layout(gtx)
	}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if !u.canRefreshWave() {
			return layout.Dimensions{}
		}
		label := "Обновить (F5)"
		if u.waveRefreshing {
			label = "Обновляем…"
		}
		return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return u.phoneButton(gtx, &u.waveRefresh, label, false) })
	}))
}
func (u *UI) cancelWaveRefresh() {
	if u.waveRefreshCancel != nil {
		u.waveRefreshCancel()
		u.waveRefreshCancel = nil
	}
	u.waveRefreshID++
	u.waveRefreshing = false
}
func (u *UI) refreshWave() {
	if u.waveRefreshing {
		return
	}
	if u.api == nil {
		u.status = "Для обновления волны войдите в Яндекс"
		return
	}
	ctx, cancel := context.WithCancel(u.ctx)
	u.waveRefreshCancel = cancel
	u.waveRefreshing = true
	u.waveRefreshID++
	id, api := u.waveRefreshID, u.waveAPI()
	seeds := u.waveSeeds()
	u.status = "Обновляем список волны…"
	go func() {
		defer cancel()
		v, e := api.WaveWithSeeds(ctx, "", nil, seeds)
		tracks := make([]music.Track, 0, len(v.Sequence))
		for _, item := range v.Sequence {
			tracks = append(tracks, item.Track)
		}
		u.emit(event{kind: "wave-refresh", waveID: id, tracks: tracks, session: v.Session, batch: v.Batch, err: e})
	}()
}
func (u *UI) waveRefreshReady(e event) {
	if e.waveID != u.waveRefreshID {
		return
	}
	u.cancelWaveRefresh()
	if e.err != nil {
		u.status = "Не удалось обновить волну: " + e.err.Error()
		return
	}
	e.tracks = u.filterRecommendations(e.tracks)
	if len(e.tracks) == 0 || e.session == "" {
		u.status = "Волна не вернула новый список"
		return
	}
	u.waveTracks = append([]music.Track(nil), e.tracks...)
	u.waveViewContexts = waveContexts(len(e.tracks), e.session, e.batch)
	u.waveViewSession, u.waveViewBatch = e.session, e.batch
	u.waveViewRefreshed = true
	if u.viewWave && u.page == "tracks" && !u.busy {
		u.setTracks(u.waveTracks)
	}
	u.status = "Список волны обновлён. Текущий трек сохранён."
}

// A refreshed list becomes the playback queue only after an explicit track tap.
func (u *UI) activateWaveView() {
	if u.waveViewSession == "" {
		return
	}
	u.cancelWaveContinuation()
	u.waveContexts = contextsForTracks(u.queue, u.waveTracks, u.waveViewContexts, u.waveViewSession, u.waveViewBatch)
	changed := u.session != u.waveViewSession
	u.session, u.batch = u.waveViewSession, u.waveViewBatch
	u.isWave = true
	u.waveViewRefreshed = false
	if changed {
		u.waveRadioStart = true
	}
}
