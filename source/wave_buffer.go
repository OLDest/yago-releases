package main

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
	"yago/internal/music"
	"yago/internal/store"
)

const waveRefillSize = 15
const waveLowWater = 5
const waveHistory = 5
const waveQueueLimit = 64
const waveMaxRequests = 8

func fetchWaveTail(ctx context.Context, api *music.API, session string, ids, seeds []string, prefs store.Preferences) ([]music.Track, []music.WaveContext, error) {
	tracks := make([]music.Track, 0, waveRefillSize)
	contexts := make([]music.WaveContext, 0, waveRefillSize)
	seen := make(map[music.ID]bool, len(ids)+waveRefillSize)
	for _, id := range ids {
		seen[music.ID(id)] = true
	}
	for request := 0; request < waveMaxRequests && len(tracks) < waveRefillSize; request++ {
		v, e := api.WaveWithSeeds(ctx, session, ids, seeds)
		if e != nil && session != "" && ctx.Err() == nil {
			v, e = api.WaveWithSeeds(ctx, "", ids, seeds)
		}
		if e != nil {
			return tracks, contexts, e
		}
		session = v.Session
		for _, item := range v.Sequence {
			t := item.Track
			if t.ID == "" || seen[t.ID] {
				continue
			}
			if len(seen) >= 256 {
				return tracks, contexts, errors.New("слишком много повторных или заблокированных рекомендаций")
			}
			seen[t.ID] = true
			ids = append(ids, string(t.ID))
			if prefs.Blocked(t) {
				continue
			}
			tracks = append(tracks, t)
			contexts = append(contexts, music.WaveContext{Session: v.Session, Batch: v.Batch})
			if len(tracks) == waveRefillSize {
				break
			}
		}
	}
	if len(tracks) < waveRefillSize {
		return tracks, contexts, errors.New("сервис пока не вернул 15 новых доступных треков")
	}
	return tracks, contexts, nil
}
func waveContexts(n int, session, batch string) []music.WaveContext {
	v := make([]music.WaveContext, n)
	for i := range v {
		v[i] = music.WaveContext{Session: session, Batch: batch}
	}
	return v
}
func contextsForTracks(tracks, source []music.Track, contexts []music.WaveContext, session, batch string) []music.WaveContext {
	out := waveContexts(len(tracks), session, batch)
	if len(source) != len(contexts) {
		return out
	}
	for i, t := range tracks {
		for j, s := range source {
			if t.ID == s.ID {
				out[i] = contexts[j]
				break
			}
		}
	}
	return out
}
func (u *UI) upcomingWave() int {
	count := 0
	for p := u.orderPos + 1; p < len(u.order); p++ {
		i := u.order[p]
		if i >= 0 && i < len(u.queue) && !u.blocked(u.queue[i]) {
			count++
		}
	}
	return count
}
func (u *UI) maybePrefetchWave() {
	if !u.isWave || u.queueOffline || u.api == nil || u.stopped || u.pauseRequested || u.current.ID == "" || u.waveNextCancel != nil || time.Now().Before(u.waveRetryAt) {
		return
	}
	if len(u.order) != len(u.queue) {
		u.resetOrder(u.index)
	}
	if u.upcomingWave() < waveLowWater {
		u.startWaveRefill()
	}
}
func (u *UI) startWaveRefill() {
	if u.waveNextCancel != nil || u.api == nil || !u.isWave || u.queueOffline || u.stopped {
		return
	}
	ctx, cancel := context.WithTimeout(u.ctx, 45*time.Second)
	u.waveNextCancel = cancel
	u.waveNextID++
	id, api := u.waveNextID, u.waveAPI()
	session := u.session
	if len(u.waveContexts) == len(u.queue) && len(u.waveContexts) > 0 {
		session = u.waveContexts[len(u.waveContexts)-1].Session
	}
	ids := make([]string, 0, min(len(u.queue), 64))
	for _, t := range u.queue[max(0, len(u.queue)-64):] {
		ids = append(ids, string(t.ID))
	}
	seeds, prefs := u.waveSeeds(), u.disc.prefs
	go func() {
		defer cancel()
		tracks, contexts, e := fetchWaveTail(ctx, api, session, ids, seeds, prefs)
		u.emit(event{kind: "wave-next", waveID: id, tracks: tracks, waveContexts: contexts, err: e})
	}()
}

// Retain a small Previous history while dropping old metadata and remapping
// shuffle indices. No audio/player operation is performed here.
func (u *UI) pruneWaveHistory() {
	cut := max(0, u.orderPos-waveHistory)
	drop := cut > 0
	if !drop {
		for _, i := range u.order[u.orderPos+1:] {
			if u.blocked(u.queue[i]) {
				drop = true
				break
			}
		}
	}
	if !drop {
		return
	}
	keep := make([]bool, len(u.queue))
	for p, i := range u.order[cut:] {
		keep[i] = p+cut <= u.orderPos || !u.blocked(u.queue[i])
	}
	remap := make([]int, len(u.queue))
	capacity := max(len(u.queue)-cut, min(waveQueueLimit, len(u.queue)-cut+waveRefillSize))
	tracks := make([]music.Track, 0, capacity)
	contexts := make([]music.WaveContext, 0, cap(tracks))
	for i, t := range u.queue {
		if keep[i] {
			remap[i] = len(tracks)
			tracks = append(tracks, t)
			contexts = append(contexts, u.waveContexts[i])
		}
	}
	order := make([]int, 0, capacity)
	for _, i := range u.order[cut:] {
		if keep[i] {
			order = append(order, remap[i])
		}
	}
	u.index = remap[u.index]
	u.orderPos -= cut
	u.queue, u.waveContexts, u.order = tracks, contexts, order
}
func (u *UI) waveContinuationReady(e event) {
	if e.waveID != u.waveNextID || !u.isWave || u.stopped {
		return
	}
	if e.kind == "wave-settings-tail" && u.current.ID != "" && len(u.filterRecommendations(e.tracks)) > 0 {
		u.queue = []music.Track{u.current}
		u.waveContexts = waveContexts(1, u.session, u.batch)
		u.index = 0
		u.order = []int{0}
		u.orderPos = 0
		u.waveViewRefreshed = false
	}
	waiting := u.waveWaiting || len(u.queue) == 0 || (u.advancePending && u.nextIndex() < 0)
	u.cancelWaveContinuation()
	if len(u.order) != len(u.queue) {
		u.resetOrder(u.index)
	}
	if len(u.waveContexts) != len(u.queue) {
		u.waveContexts = waveContexts(len(u.queue), u.session, u.batch)
	}
	if len(e.waveContexts) != len(e.tracks) {
		e.waveContexts = waveContexts(len(e.tracks), e.session, e.batch)
	}
	seen := make(map[music.ID]bool, len(u.queue)+len(e.tracks))
	for _, t := range u.queue {
		seen[t.ID] = true
	}
	tracks := make([]music.Track, 0, len(e.tracks))
	contexts := make([]music.WaveContext, 0, len(e.tracks))
	for i, t := range e.tracks {
		if t.ID != "" && !seen[t.ID] && !u.blocked(t) {
			seen[t.ID] = true
			tracks = append(tracks, t)
			contexts = append(contexts, e.waveContexts[i])
		}
	}
	if len(tracks) == 0 {
		u.waveRetryAt = time.Now().Add(30 * time.Second)
		if waiting {
			u.advancePending = true
		}
		if u.viewWave || waiting || e.kind == "wave-settings-tail" {
			u.status = "Не удалось пополнить волну"
			if e.err != nil {
				u.status += ": " + e.err.Error()
			}
		}
		return
	}
	if len(u.queue) > 0 && u.index >= 0 && u.index < len(u.queue) {
		u.pruneWaveHistory()
	}
	room := max(0, waveQueueLimit-len(u.queue))
	if len(tracks) > room {
		tracks = tracks[:room]
		contexts = contexts[:room]
	}
	first := len(u.queue)
	u.queue = append(u.queue, tracks...)
	u.waveContexts = append(u.waveContexts, contexts...)
	oldOrder := len(u.order)
	for i := range tracks {
		u.order = append(u.order, first+i)
	}
	if u.shuffled {
		rand.Shuffle(len(tracks), func(i, j int) { u.order[oldOrder+i], u.order[oldOrder+j] = u.order[oldOrder+j], u.order[oldOrder+i] })
	}
	if !u.waveViewRefreshed {
		u.waveTracks = u.queue
		u.waveViewContexts = u.waveContexts
		u.waveViewSession, u.waveViewBatch = u.session, u.batch
		if u.viewWave && u.page == "tracks" && !u.busy {
			offset := u.fit.offset
			u.setTracks(u.filterRecommendations(u.queue))
			u.fit.offset = offset
		}
	}
	if e.err != nil {
		u.waveRetryAt = time.Now().Add(30 * time.Second)
	}
	if waiting {
		next := u.nextIndex()
		if first == 0 {
			next = u.order[0]
		}
		if next >= 0 {
			u.startAt(next, 0, u.pauseRequested, true)
		}
	} else {
		if e.kind == "wave-settings-tail" && e.err == nil {
			u.status = "Настройки обоих сервисов применены. Следующие треки обновлены."
		}
		u.savePlayback(true)
	}
}
