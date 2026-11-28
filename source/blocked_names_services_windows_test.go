package main

import (
	"testing"
	"yago/internal/music"
	"yago/internal/store"
)

func TestBlockedNamesServiceStep(t *testing.T) {
	u := serviceTestUI(t)
	for _, p := range []store.Preferences{
		{Owner: "11", Tracks: map[music.ID]bool{"123": true}, TrackNames: map[music.ID]string{"123": "Yandex artist — Song"}},
		{Owner: "zvuk-22", Tracks: map[music.ID]bool{"zvuk-123": true}, TrackNames: map[music.ID]string{"zvuk-123": "Zvuk artist — Song"}},
	} {
		if e := store.SavePreferences(stateDir, p); e != nil {
			t.Fatal(e)
		}
	}
	u.services.mode = "all"
	p, e := u.loadServicePreferences("11")
	if e != nil {
		t.Fatal(e)
	}
	if len(p.TrackNames) != 2 || p.TrackNames["123"] != "Yandex artist — Song" || p.TrackNames["zvuk-123"] != "Zvuk artist — Song" {
		t.Fatalf("merged names lost: %+v", p)
	}
	if e := u.saveServicePreferences(p); e != nil {
		t.Fatal(e)
	}
	y, e := store.LoadPreferences(stateDir, "11")
	if e != nil {
		t.Fatal(e)
	}
	z, e := store.LoadPreferences(stateDir, "zvuk-22")
	if e != nil {
		t.Fatal(e)
	}
	if len(y.TrackNames) != 1 || y.TrackNames["123"] == "" || len(z.TrackNames) != 1 || z.TrackNames["zvuk-123"] == "" {
		t.Fatalf("cross-account names: %+v / %+v", y, z)
	}
	delete(p.Tracks, "zvuk-123") // stale name must not survive an unblock when splitting
	if e := u.saveServicePreferences(p); e != nil {
		t.Fatal(e)
	}
	z, e = store.LoadPreferences(stateDir, "zvuk-22")
	if e != nil {
		t.Fatal(e)
	}
	if len(z.Tracks) != 0 || len(z.TrackNames) != 0 {
		t.Fatalf("unblocked metadata retained: %+v", z)
	}
	again, e := u.loadServicePreferences("11")
	if e != nil || len(again.TrackNames) != 1 || again.TrackNames["123"] == "" {
		t.Fatalf("reload: %+v %v", again, e)
	}
	u.services.mode = "yandex"
	single, e := u.loadServicePreferences("11")
	if e != nil || single.TrackNames["123"] == "" {
		t.Fatal("single service name lost", e)
	}
}
