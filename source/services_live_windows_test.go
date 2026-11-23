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

func TestLiveCombinedLibrary(t *testing.T) {
	state := os.Getenv("YAGO_LIVE_LIBRARY_STATE")
	if state == "" {
		t.Skip("explicit saved-account library check")
	}
	device, e := os.ReadFile(filepath.Join(state, "device-id"))
	if e != nil || len(device) != 32 {
		t.Fatal("device unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r := &music.Services{}
	for _, service := range []string{"yandex", "zvuk"} {
		token, e := store.Load(credentialKey(string(device), service))
		if e != nil {
			t.Fatal("saved account unavailable", service)
		}
		api := music.New(token.Access)
		if service == "zvuk" {
			api = newZvukAPI(token)
		}
		account, e := api.Account(ctx)
		if e != nil {
			t.Fatal(service, "account", e)
		}
		refs, e := api.History(ctx)
		if e != nil {
			t.Error(service, "history", e)
		}
		likes, moreLikes, e := api.LikesPage(ctx, account.UID, 0)
		if e != nil {
			t.Error(service, "likes", e)
		}
		lists, e := api.Playlists(ctx, account.UID)
		if e != nil {
			t.Error(service, "playlists", e)
		}
		t.Logf("%s: history=%d favorites first page=%d more=%v playlists=%d", service, len(refs), len(likes), moreLikes, len(lists))
		if service == "yandex" {
			r.Yandex = api
			r.YandexAccount = account
		} else {
			r.Zvuk = api
			r.ZvukAccount = account
		}
	}
	refs, e := r.History(ctx)
	if e != nil {
		t.Fatal("combined history", e)
	}
	tracks, _, e := r.HistoryTracks(ctx, refs, 0)
	if e != nil {
		t.Fatal("combined history page", e)
	}
	y, z := 0, 0
	for _, tr := range tracks {
		if music.IsZvuk(tr.ID) {
			z++
		} else {
			y++
		}
	}
	t.Logf("Combined history first page: Yandex=%d Zvuk=%d", y, z)
	favorites, _, e := r.LikesPage(ctx, "all", 0)
	if e != nil {
		t.Fatal(e)
	}
	y, z = 0, 0
	for _, tr := range favorites {
		if music.IsZvuk(tr.ID) {
			z++
		} else {
			y++
		}
	}
	t.Logf("Combined favorites first page: Yandex=%d Zvuk=%d", y, z)
}
