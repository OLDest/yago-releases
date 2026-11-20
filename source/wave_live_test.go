package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"yago/internal/music"
	"yago/internal/store"
)

func TestLiveWaveFetch15(t *testing.T) {
	state := os.Getenv("YAGO_LIVE_WAVE_STATE")
	if state == "" {
		t.Skip("explicit saved-account Wave verification")
	}
	device, e := os.ReadFile(filepath.Join(state, "device-id"))
	if e != nil || len(device) != 32 {
		t.Fatal("device unavailable")
	}
	token, e := store.Load(string(device))
	if e != nil {
		t.Fatal("Yandex account unavailable")
	}
	api := music.New(token.Access)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	account, e := api.Account(ctx)
	if e != nil {
		t.Fatal(e)
	}
	prefs, e := store.LoadPreferences(state, account.UID)
	if e != nil {
		t.Fatal(e)
	}
	tracks, contexts, e := fetchWaveTail(ctx, api, "", nil, nil, prefs)
	if e != nil {
		t.Fatal(e)
	}
	if len(tracks) != 15 || len(contexts) != 15 {
		t.Fatal("not 15 tracks")
	}
	seen := map[music.ID]bool{}
	for i, tr := range tracks {
		if seen[tr.ID] || contexts[i].Session == "" || prefs.Blocked(tr) {
			t.Fatal("invalid tail")
		}
		seen[tr.ID] = true
	}
	t.Log("Live Yandex Wave: fetched 15 unique allowed tracks with per-track session context")
}
