package main

import (
	"context"
	"errors"
	"fmt"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"yago/internal/music"
)

func qualityModeName(q music.Quality) string {
	if q == music.Lossless {
		return "FLAC"
	}
	if q == music.Normal {
		return "Basic"
	}
	return "HQ"
}
func (u *UI) initPlaybackSettings() {
	u.quality = music.High
	if b, e := os.ReadFile(filepath.Join(stateDir, "quality")); e == nil {
		if n, _ := strconv.Atoi(string(b)); n == 192 || n == int(music.Lossless) {
			u.quality = music.Quality(n)
		}
	}
}
func (u *UI) qualityButton(gtx layout.Context) layout.Dimensions {
	if music.IsMusify(u.current.ID) || (u.current.ID == "" && u.services.mode == "musify") {
		return u.label(gtx, "Качество: авто", 12, muted)
	}

	label := qualityModeName(u.quality) + "  "
	if u.qualitySwitching {
		label = qualityModeName(u.pendingQuality) + " …  "
	}
	d := u.button(gtx, &u.qualityClick, label, false)
	var arrow clip.Path
	arrow.Begin(gtx.Ops)
	x, y := float32(d.Size.X-gtx.Dp(12)), float32(d.Size.Y/2-gtx.Dp(2))
	arrow.MoveTo(f32.Pt(x, y))
	arrow.LineTo(f32.Pt(x+float32(gtx.Dp(6)), y))
	arrow.LineTo(f32.Pt(x+float32(gtx.Dp(3)), y+float32(gtx.Dp(4))))
	arrow.Close()
	paint.FillShape(gtx.Ops, white, clip.Outline{Path: arrow.End()}.Op())
	if !u.qualityMenuOpen {
		return d
	}
	macro := op.Record(gtx.Ops)
	shield := gtx
	shield.Constraints = layout.Exact(image.Pt(20000, 20000))
	shift := op.Offset(image.Pt(-10000, -10000)).Push(gtx.Ops)
	u.qualityDismiss.Layout(shield, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} })
	shift.Pop()
	menu := gtx
	menu.Constraints.Min = image.Point{}
	menu.Constraints.Max = image.Pt(gtx.Dp(190), gtx.Dp(240))
	menu.Constraints.Min.X = menu.Constraints.Max.X
	body := op.Record(gtx.Ops)
	dimensions := inset(6, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.qualityBasic, u.basicQualityLabel(), u.quality == music.Normal)
			}), gap(4),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.qualityHQ, "HQ · лучший MP3", u.quality == music.High)
			}), gap(4), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.qualityFLAC, "FLAC · без потерь", u.quality == music.Lossless)
			}))
	})(menu)
	content := body.Stop()
	at := op.Offset(image.Pt(min(0, d.Size.X-dimensions.Size.X), -dimensions.Size.Y-gtx.Dp(6))).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, panel, clip.Rect{Max: dimensions.Size}.Op())
	content.Add(gtx.Ops)
	border := widget.Border{Color: muted, Width: 1, CornerRadius: 8}
	border.Layout(menu, func(layout.Context) layout.Dimensions { return dimensions })
	at.Pop()
	op.Defer(gtx.Ops, macro.Stop())
	return d
}
func (u *UI) cancelQualityChange() {
	if u.qualityCancel != nil {
		u.qualityCancel()
		u.qualityCancel = nil
	}
	u.qualitySequence++
	u.qualitySwitching = false
}
func (u *UI) selectQuality(q music.Quality) {
	if q != music.Normal && q != music.Lossless {
		q = music.High
	}
	u.qualityMenuOpen = false
	u.cancelQualityChange()
	if u.queueOffline {
		u.quality = q
		if e := os.WriteFile(filepath.Join(stateDir, "quality"), []byte(strconv.Itoa(int(q))), 0600); e != nil {
			u.status = "Не удалось сохранить качество"
			return
		}
		u.status = "Качество " + qualityModeName(q) + " — для новых загрузок. Локальный файл не меняется."
		return
	}
	if u.player == nil || u.current.ID == "" || u.index < 0 || u.index >= len(u.queue) {
		u.applyQuality(q, nil)
		return
	}
	if demo || u.api == nil {
		u.applyQuality(q, nil)
		return
	}
	ctx, cancel := context.WithCancel(u.ctx)
	u.qualityCancel = cancel
	u.qualitySwitching = true
	u.pendingQuality = q
	sequence, version, track, api := u.qualitySequence, u.streamVersion, u.current, u.api
	u.status = "Проверяем " + qualityModeName(q) + "…"
	go func() {
		defer cancel()
		info, e := api.Stream(ctx, track, q)
		u.emit(event{kind: "quality-ready", qualitySequence: sequence, streamID: version, quality: q, streamInfo: info, err: e})
	}()
}
func (u *UI) qualityReady(e event) {
	if e.qualitySequence != u.qualitySequence || e.streamID != u.streamVersion {
		return
	}
	if u.qualityCancel != nil {
		u.qualityCancel()
		u.qualityCancel = nil
	}
	u.qualitySwitching = false
	if e.err != nil {
		if errors.Is(e.err, music.ErrQualityUnavailable) {
			u.status = "Basic: MP3 192 недоступен для этого трека. Текущее воспроизведение сохранено."
		} else {
			u.status = "Не удалось сменить качество: " + e.err.Error()
		}
		return
	}
	u.applyQuality(e.quality, &e.streamInfo)
}
func (u *UI) applyQuality(q music.Quality, info *music.StreamInfo) {
	u.quality = q
	if e := os.WriteFile(filepath.Join(stateDir, "quality"), []byte(strconv.Itoa(int(q))), 0600); e != nil {
		u.status = "Не удалось сохранить режим качества"
	}
	if u.player == nil || u.current.ID == "" || u.index < 0 || u.index >= len(u.queue) {
		u.status = "Режим " + qualityModeName(q) + " сохранён"
		return
	}
	s := u.player.Snapshot()
	offset := u.requestedOffset
	if s.ID == u.playID {
		offset = s.Position
	}
	paused := u.pauseRequested || u.stopped || s.Ended
	u.startPrepared(u.index, offset, paused, false, info)
	u.reloadingBest = true
	u.status = "Загружаем этот трек: " + qualityModeName(q) + "…"
}
func (u *UI) applyVolume() {
	v := u.volume
	if u.muted {
		v = 0
	}
	u.player.Volume(float64(v))
}
func (u *UI) changeVolume(delta float32) {
	u.volume = max(0, min(1, u.volume+delta))
	u.muted = false
	u.applyVolume()
}
func (u *UI) muteVolume() { u.muted = !u.muted; u.applyVolume() }
func (u *UI) volumeLabel() string {
	if u.muted {
		return "MUTE"
	}
	return fmt.Sprintf("%d%%", int(u.volume*100+.5))
}
func (u *UI) streamLabel() string {
	if u.streamCodec == "flac" {
		return " · FLAC"
	}
	if u.streamBitrate > 0 {
		return fmt.Sprintf(" · MP3 %d кбит/с", u.streamBitrate)
	}
	return ""
}
func (u *UI) resumePlayback() {
	if u.resumeSuspended() {
		return
	}
	u.pauseRequested = false
	if len(u.queue) == 0 && len(u.tracks) > 0 {
		u.queueOffline = u.page == "offline-tracks"
		if u.queueOffline {
			u.offlineQueuePlaylist = u.off.view
		} else {
			u.offlineQueuePlaylist = ""
		}
		u.queue = append([]music.Track(nil), u.tracks...)
		u.resetOrder(0)
	}
	s := u.player.Snapshot()
	if u.stopped || s.ID == 0 || s.Ended || s.Error != "" {
		if u.current.ID != "" {
			u.start(u.index)
		} else if len(u.queue) > 0 {
			u.start(0)
		}
	} else if !s.Playing {
		u.player.Resume()
	}
}

func (u *UI) finishPlaybackLoad() {
	if !u.reloadingBest && !u.restoringPlayback {
		return
	}
	s := u.player.Snapshot()
	if s.ID != u.playID || s.Loading {
		return
	}
	if s.Error == "" {
		if u.reloadingBest {
			u.status = "Трек перезагружен: " + qualityModeName(u.quality)
			if u.streamBitrate > 0 {
				u.status = fmt.Sprintf("%s: фактически MP3 %d кбит/с", qualityModeName(u.quality), u.streamBitrate)
			}
		} else {
			u.status = "Воспроизведение восстановлено"
		}
	}
	u.reloadingBest = false
	u.restoringPlayback = false
}
