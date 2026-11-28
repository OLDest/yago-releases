package store

import (
	"encoding/json"
	"strings"
	"testing"
	"yago/internal/music"
)

func TestBlockedNamesStepRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := Preferences{Owner: "owner", Tracks: map[music.ID]bool{"zvuk-12": true}, TrackNames: map[music.ID]string{"zvuk-12": "Artist — Song"}}
	if e := SavePreferences(dir, p); e != nil {
		t.Fatal(e)
	}
	q, e := LoadPreferences(dir, p.Owner)
	if e != nil {
		t.Fatal(e)
	}
	if q.TrackNames["zvuk-12"] != "Artist — Song" || !q.Blocked(music.Track{ID: "zvuk-12"}) {
		t.Fatalf("lost data: %+v", q)
	}
	var legacy Preferences
	if e := json.Unmarshal([]byte(`{"owner":"old","tracks":{"123":true}}`), &legacy); e != nil {
		t.Fatal(e)
	}
	if e := legacy.Validate(); e != nil {
		t.Fatal(e)
	}
	if !legacy.Blocked(music.Track{ID: "123"}) {
		t.Fatal("legacy block lost")
	}
}
func TestBlockedNamesStepBounds(t *testing.T) {
	for _, names := range []map[music.ID]string{{"": "bad"}, {"id": strings.Repeat("x", 513)}} {
		if (Preferences{Owner: "owner", TrackNames: names}).Validate() == nil {
			t.Fatal("invalid name accepted")
		}
	}
}
