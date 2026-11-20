package store

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"yago/internal/music"
)

func validPlayback() Playback {
	return Playback{Version: 1, Owner: "42", Queue: []music.Track{{ID: "a"}, {ID: "b"}}, Order: []int{1, 0}, Index: 1, Position: 12.5, Playing: true, Volume: .65, Shuffled: true}
}
func TestPlaybackRoundTripAndAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	p := validPlayback()
	if e := SavePlayback(dir, p); e != nil {
		t.Fatal(e)
	}
	got, e := LoadPlayback(dir)
	if e != nil || got.Queue[got.Index].ID != "b" || got.Position != 12.5 || !got.Playing || got.Order[0] != 1 {
		t.Fatalf("%+v %v", got, e)
	}
	p.Position = 15
	p.Playing = false
	if e = SavePlayback(dir, p); e != nil {
		t.Fatal(e)
	}
	got, e = LoadPlayback(dir)
	if e != nil || got.Position != 15 || got.Playing {
		t.Fatal(got, e)
	}
	p.Position = math.NaN()
	if SavePlayback(dir, p) == nil {
		t.Fatal("NaN accepted")
	}
	got, e = LoadPlayback(dir)
	if e != nil || got.Position != 15 {
		t.Fatal("invalid write damaged previous checkpoint")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".playback-*"))
	if len(leftovers) > 0 {
		t.Fatal("temporary file retained")
	}
}

func TestWavePlaybackKeepsPerTrackSessionAndBatch(t *testing.T) {
	dir := t.TempDir()
	p := validPlayback()
	p.Wave = true
	p.Session = "active"
	p.Batch = "batch-active"
	p.WaveContexts = []music.WaveContext{{Session: "first", Batch: "batch-first"}, {Session: "active", Batch: "batch-active"}}
	if e := SavePlayback(dir, p); e != nil {
		t.Fatal(e)
	}
	got, e := LoadPlayback(dir)
	if e != nil || len(got.WaveContexts) != 2 || got.WaveContexts[0].Batch != "batch-first" || got.WaveContexts[1].Session != "active" {
		t.Fatal(got, e)
	}
	p.WaveContexts = p.WaveContexts[:1]
	if p.Validate() == nil {
		t.Fatal("misaligned contexts accepted")
	}
	p.WaveContexts = nil
	if e = p.Validate(); e != nil {
		t.Fatal("legacy checkpoint rejected", e)
	}
}
func TestPlaybackRejectsCorruption(t *testing.T) {
	for _, kind := range []string{"version", "index", "order", "owner", "position", "track"} {
		t.Run(kind, func(t *testing.T) {
			p := validPlayback()
			switch kind {
			case "version":
				p.Version = 9
			case "index":
				p.Index = 2
			case "order":
				p.Order = []int{0, 0}
			case "owner":
				p.Owner = ""
			case "position":
				p.Position = -1
			case "track":
				p.Queue[0].ID = ""
			}
			if p.Validate() == nil {
				t.Fatal("invalid checkpoint accepted")
			}
		})
	}
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "playback.json"), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadPlayback(dir); e == nil {
		t.Fatal("broken JSON accepted")
	}
	if e := DeletePlayback(dir); e != nil {
		t.Fatal(e)
	}
	if e := DeletePlayback(dir); e != nil {
		t.Fatal(e)
	}
}
