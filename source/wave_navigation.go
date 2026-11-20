package main

import "time"

func (u *UI) openWave() {

	u.section = sectionWave
	tracks := u.waveTracks
	if !u.waveViewRefreshed && u.isWave && len(u.queue) > 0 {
		tracks = u.queue
	}
	tracks = u.filterRecommendations(tracks)
	if !u.waveViewRefreshed && u.isWave {
		u.waveViewContexts = contextsForTracks(tracks, u.queue, u.waveContexts, u.session, u.batch)
	}
	if len(tracks) > 0 {
		loadingWave := u.busy && u.title == u.waveTitle()
		if !loadingWave {
			u.cancelLibrary()
			u.gen++
			u.busy = false
		}
		u.up.show = false
		u.off.removing = ""
		u.page = "tracks"
		u.viewWave = true
		u.title = u.waveTitle()
		u.setTracks(tracks)
		u.status = u.waveTitle()
		if loadingWave {
			u.status = "Подгружаем продолжение волны…"
		}
		return
	}
	if u.api == nil {
		u.status = "Для волны подключите профиль"
		return
	}
	if u.busy && u.title == u.waveTitle() {
		return
	}
	// First load starts playback only after the server supplies a valid batch.
	// Returning to an existing Wave never sends skip or restarts the player.
	u.loadWave(true)
}

func (u *UI) cancelWaveContinuation() {
	if u.waveNextCancel != nil {
		u.waveNextCancel()
		u.waveNextCancel = nil
	}
	u.waveNextID++
	u.waveWaiting = false
	u.waveRetryAt = time.Time{}
}
func (u *UI) continueWave() {
	if u.api == nil || !u.isWave || u.stopped {
		return
	}
	u.waveWaiting = true
	u.advancePending = true
	u.startWaveRefill()
}
