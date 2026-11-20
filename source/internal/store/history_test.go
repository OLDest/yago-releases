package store

import (
	"fmt"
	"testing"
	"time"
	"yago/internal/music"
)

func TestLocalHistoryBoundedDeduplicatedAndAccountScoped(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for i := 0; i < 205; i++ {
		if e := RecordHistory(dir, "zvuk-owner", music.Track{ID: music.ID(fmt.Sprint(i)), CoverURI: "unused-image"}, now); e != nil {
			t.Fatal(e)
		}
	}
	items, e := LoadHistory(dir, "zvuk-owner")
	if e != nil || len(items) != HistoryLimit || items[0].Track.ID != "204" || items[0].Track.CoverURI != "" {
		t.Fatal("history bounds", e)
	}
	if e := RecordHistory(dir, "zvuk-owner", music.Track{ID: "10"}, now); e != nil {
		t.Fatal(e)
	}
	items, _ = LoadHistory(dir, "zvuk-owner")
	count := 0
	for _, h := range items {
		if h.Track.ID == "10" {
			count++
		}
	}
	if count != 1 || items[0].Track.ID != "10" {
		t.Fatal("history duplicate")
	}
	other, _ := LoadHistory(dir, "other-owner")
	if len(other) != 0 {
		t.Fatal("history crossed accounts")
	}
}
