package main

import (
	"context"
	"testing"
	"yago/internal/audio"
	"yago/internal/music"
	"yago/internal/store"
)

func TestProfileLoginPreservesWaveAndPendingLibrary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	library, closeLibrary := context.WithCancel(ctx)
	defer closeLibrary()
	refill, closeRefill := context.WithCancel(ctx)
	defer closeRefill()
	u := &UI{ctx: ctx, gen: 7, viewWave: true, isWave: true, page: "tracks", title: "Моя волна", session: "s", batch: "b", libraryCancel: closeLibrary, waveNextCancel: closeRefill, playID: 42, streamVersion: 8}
	_, closeLogin := u.prepareProfileLogin()
	defer closeLogin()
	if !u.viewWave || !u.isWave || u.gen != 7 || u.playID != 42 || u.streamVersion != 8 || library.Err() != nil || refill.Err() != nil {
		t.Fatal("login cancelled/downgraded Wave")
	}
	_, closeNext := u.prepareProfileLogin()
	defer closeNext()
	if library.Err() != nil || refill.Err() != nil {
		t.Fatal("retry cancelled Wave fetch")
	}
}
func TestWaveCheckpointAfterProfileLogin(t *testing.T) {
	withPlaybackDemo(t)
	p := audio.New(t.TempDir(), nil)
	defer p.Close()
	u := &UI{ctx: context.Background(), player: p, isWave: true, viewWave: true, queue: waveTracksForTest(0, 5), index: 0, current: music.Track{ID: "0"}, session: "s", batch: "b", volume: .5}
	u.resetOrder(0)
	u.waveContexts = waveContexts(5, "s", "b")
	_, cancel := u.prepareProfileLogin()
	defer cancel()
	u.savePlayback(true)
	saved, e := store.LoadPlayback(stateDir)
	if e != nil || !saved.Wave || saved.Session != "s" || len(saved.WaveContexts) != 5 {
		t.Fatal("Wave persisted as fixed queue", e)
	}
	v := &UI{ctx: context.Background(), player: p, pendingPlayback: &saved, index: -1}
	v.restorePlayback()
	if !v.isWave || !v.viewWave || len(v.waveContexts) != 5 {
		t.Fatal("Wave lost after restart")
	}
}
