package store

import (
	"os"
	"testing"
	"yago/internal/music"
)

func TestPreferencesPersistAndSeparateAccounts(t *testing.T) {
	dir := t.TempDir()
	p := Preferences{Owner: "one", Artists: map[music.ID]string{"artist": "Name"}, Tracks: map[music.ID]bool{"song": true}, Seeds: map[string]string{"diversity": "settingDiversity:discover"}}
	if e := SavePreferences(dir, p); e != nil {
		t.Fatal(e)
	}
	got, e := LoadPreferences(dir, "one")
	if e != nil || got.Artists["artist"] != "Name" || !got.Blocked(music.Track{ID: "song"}) || !got.Blocked(music.Track{ID: "collaboration", Artists: []music.Named{{ID: "other"}, {ID: "artist"}}}) {
		t.Fatal(got, e)
	}
	other, e := LoadPreferences(dir, "two")
	if e != nil || other.Blocked(music.Track{ID: "song"}) {
		t.Fatal(other, e)
	}
	delete(got.Artists, "artist")
	if e = SavePreferences(dir, got); e != nil {
		t.Fatal(e)
	}
	got, e = LoadPreferences(dir, "one")
	if e != nil || got.Blocked(music.Track{ID: "new", Artists: []music.Named{{ID: "artist"}}}) {
		t.Fatal(got, e)
	}
	if e = os.WriteFile(preferenceFile(dir, "two"), []byte(`{"owner":"one","artists":{"artist":"Name"}}`), 0600); e != nil {
		t.Fatal(e)
	}
	other, e = LoadPreferences(dir, "two")
	if e == nil || len(other.Artists) > 0 {
		t.Fatal("foreign preferences accepted")
	}
}
