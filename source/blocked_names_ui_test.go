package main

import (
	"strings"
	"testing"
	"unicode/utf8"
	"yago/internal/music"
	"yago/internal/store"
)

func TestBlockedNamesUIStep(t *testing.T) {
	u, _, track := feedbackFixture(t)
	track.Title = "Song"
	track.Artists = []music.Named{{ID: "artist", Name: "Artist"}}
	u.moderateTrack(menuEntry{Action: "dislike", ID: track.ID}, track)
	feedbackDone(t, u)
	p, e := store.LoadPreferences(stateDir, "owner")
	if e != nil {
		t.Fatal(e)
	}
	if p.TrackNames[track.ID] != "Artist — Song" || !p.Tracks[track.ID] {
		t.Fatalf("name not saved: %+v", p)
	}
	u.disc.prefs = p
	u.tracks = nil
	u.queue = nil
	u.blockedMenu(0)
	found := false
	for _, entry := range u.disc.entries {
		if entry.ID == track.ID {
			found = true
			if entry.Label != "Разрешить трек Artist — Song" || entry.Action != "allow-track" {
				t.Fatal(entry)
			}
		}
	}
	if !found {
		t.Fatal("missing blocked entry")
	}
	u.moderateTrack(menuEntry{Action: "allow-track", ID: track.ID}, music.Track{ID: track.ID})
	feedbackDone(t, u)
	p, e = store.LoadPreferences(stateDir, "owner")
	if e != nil || p.Tracks[track.ID] || p.TrackNames[track.ID] != "" {
		t.Fatal("unblock retained name", p, e)
	}
}
func TestBlockedNamesLegacyAndUnicodeStep(t *testing.T) {
	u := &UI{}
	u.disc.prefs = store.Preferences{Owner: "owner", Tracks: map[music.ID]bool{"known": true, "legacy-unknown": true}}
	u.tracks = []music.Track{{ID: "known", Title: "Known title"}}
	u.blockedMenu(0)
	labels := map[music.ID]string{}
	for _, entry := range u.disc.entries {
		labels[entry.ID] = entry.Label
	}
	if labels["known"] != "Разрешить трек Known title" || labels["legacy-unknown"] != "Разрешить трек legacy-unknown" {
		t.Fatal(labels)
	}
	label := blockedTrackDisplayName(music.Track{Title: strings.Repeat("Я", 400)}, "")
	if len(label) > 512 || !utf8.ValidString(label) || label == "" {
		t.Fatal("invalid bounded Unicode label")
	}
	if blockedTrackDisplayName(music.Track{}, "") != "" {
		t.Fatal("fabricated title")
	}
}
