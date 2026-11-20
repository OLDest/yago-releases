package main

import (
	"errors"
	"log"
	"math"
	"os"
	"time"
	"yago/internal/music"
	"yago/internal/offline"
	"yago/internal/store"
)

type checkpointKey struct {
	ID                               uint64
	Track                            music.ID
	Index, Count                     int
	Second                           int64
	Paused, Stopped, Muted, Shuffled bool
	Volume                           float32
}

func (u *UI) initPlaybackRestore() {
	p, e := store.LoadPlayback(stateDir)
	if e != nil {
		if !errors.Is(e, os.ErrNotExist) {
			log.Print("Playback checkpoint could not be loaded")
		}
		return
	}
	if !p.Offline && p.Demo != demo {
		return
	}
	u.pendingPlayback = &p
	if demo || p.Offline {
		u.restorePlayback()
	}
}
func (u *UI) restorePlayback() {
	p := u.pendingPlayback
	if p == nil {
		return
	}
	if !p.Offline && !p.Demo && (u.api == nil || u.account.UID == "") {
		return
	}
	u.pendingPlayback = nil
	if !p.Offline && (p.Demo != demo || (!demo && p.Owner != u.account.UID)) {
		return
	}
	if u.blocked(p.Queue[p.Index]) {
		next := -1
		for _, i := range p.Order {
			if !u.blocked(p.Queue[i]) {
				next = i
				break
			}
		}
		if next < 0 {
			u.status = "В сохранённой очереди все треки заблокированы"
			return
		}
		p.Index = next
		p.Position = 0
	}
	u.queue = append([]music.Track(nil), p.Queue...)
	u.waveContexts = nil
	if p.Wave {
		u.restoreWaveSource(p.Queue[p.Index].ID)
		u.waveContexts = contextsForTracks(u.queue, p.Queue, p.WaveContexts, p.Session, p.Batch)
	}
	u.queueOffline = p.Offline
	u.order = append([]int(nil), p.Order...)
	u.index = p.Index
	u.current = u.queue[u.index]
	for i, v := range u.order {
		if v == u.index {
			u.orderPos = i
			break
		}
	}
	u.shuffled = p.Shuffled
	u.volume = p.Volume
	u.muted = p.Muted
	u.pauseRequested = !p.Playing
	u.stopped = p.Stopped
	u.isWave = p.Wave
	u.session = p.Session
	u.batch = p.Batch
	u.page = "tracks"
	u.viewWave = p.Wave
	if p.Offline {
		u.section = sectionOffline
		u.page = "offline-tracks"
		u.off.view = p.OfflinePlaylist
		if u.off.view == "" {
			u.off.view = offline.DefaultID
		}
		u.offlineQueuePlaylist = u.off.view
		demo = false
	}
	u.title = "Продолжить прослушивание"
	if p.Offline {
		u.title = u.offlineName(u.off.view)
	}
	if p.Wave {
		u.waveViewSession, u.waveViewBatch = p.Session, p.Batch
		u.viewWave = true
		u.waveTracks = append([]music.Track(nil), u.queue...)
		u.waveViewContexts = u.waveContexts
		u.title = u.waveTitle()
	}
	if p.Wave {
		u.setTracks(u.filterRecommendations(u.queue))
	} else {
		u.setTracks(u.queue)
	}
	u.applyVolume()
	if p.Stopped {
		u.requestedOffset = 0
		u.status = "Воспроизведение восстановлено: остановлено"
		return
	}
	u.startAt(p.Index, p.Position, !p.Playing, false)
	u.restoringPlayback = true
	u.status = "Восстанавливаем трек и позицию…"
}
func (u *UI) savePlayback(force bool) {
	if u.checkpointSuspended {
		return
	}
	if u.player == nil || u.current.ID == "" || len(u.queue) == 0 || u.index < 0 || u.index >= len(u.queue) {
		return
	}
	owner := u.account.UID
	if demo {
		owner = "__demo__"
	}
	if u.queueOffline {
		owner = "__offline__"
	}
	if owner == "" {
		return
	}
	if !force && time.Since(u.lastCheckpoint) < 3*time.Second {
		return
	}
	s := u.player.Snapshot()
	position := u.requestedOffset
	ended := false
	if s.ID == u.playID {
		position = s.Position
		ended = s.Ended
	}
	stopped := u.stopped || ended
	if stopped {
		position = 0
	}
	if math.IsNaN(position) || math.IsInf(position, 0) || position < 0 {
		position = 0
	}
	if duration := float64(u.current.DurationMS) / 1000; duration > 0 {
		position = min(position, max(0, duration-.25))
	}
	position = min(position, 86400)
	key := checkpointKey{u.playID, u.current.ID, u.index, len(u.queue), int64(position), u.pauseRequested, stopped, u.muted, u.shuffled, u.volume}
	if !force && u.hasCheckpoint && key == u.lastCheckpointKey {
		return
	}
	if len(u.order) != len(u.queue) {
		u.resetOrder(u.index)
	}
	playing := !u.pauseRequested && !stopped
	if s.ID == u.playID && s.Error != "" {
		playing = false
	}
	p := store.Playback{Version: 1, Owner: owner, Demo: demo && !u.queueOffline, Offline: u.queueOffline, OfflinePlaylist: u.offlineQueuePlaylist, Queue: u.queue, Order: u.order, Index: u.index, Position: position, Playing: playing, Stopped: stopped, Volume: u.volume, Muted: u.muted, Shuffled: u.shuffled, Wave: u.isWave && !u.queueOffline, Session: u.session, Batch: u.batch}
	if p.Wave {
		p.WaveContexts = u.waveContexts
	}
	if e := store.SavePlayback(stateDir, p); e != nil {
		if !u.checkpointError {
			log.Print("Playback checkpoint could not be saved")
			u.status = "Не удалось сохранить воспроизведение"
		}
		u.checkpointError = true
		return
	}
	u.lastCheckpoint = time.Now()
	u.lastCheckpointKey = key
	u.hasCheckpoint = true
	u.checkpointError = false
}
func (u *UI) clearPlayback() {
	u.cancelWaveContinuation()
	u.pendingPlayback = nil
	u.checkpointSuspended = false
	u.hasCheckpoint = false
	u.streamVersion++
	if e := store.DeletePlayback(stateDir); e != nil {
		log.Print("Playback checkpoint could not be removed")
	}
}

func (u *UI) suspendPlayback() {
	u.cancelWaveContinuation()
	u.savePlayback(true)
	s := u.player.Snapshot()
	if s.ID == u.playID {
		u.requestedOffset = s.Position
	}
	u.checkpointSuspended = true
	u.stopped = true
	u.advancePending = true
	u.serviceActive = false
	u.player.Stop()
}
func (u *UI) resumeSuspended() bool {
	if !u.checkpointSuspended {
		return false
	}
	u.checkpointSuspended = false
	p, e := store.LoadPlayback(stateDir)
	if e != nil {
		return false
	}
	u.pendingPlayback = &p
	u.restorePlayback()
	if u.stopped {
		return false
	}
	u.pauseRequested = false
	u.player.Resume()
	return true
}
