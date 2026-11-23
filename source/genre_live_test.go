package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"yago/internal/music"
	"yago/internal/store"
	"yago/internal/zvuk"
)

func TestLiveYandexGenreWave(t *testing.T) {
	state := os.Getenv("YAGO_LIVE_WAVE_STATE")
	if state == "" {
		t.Skip("explicit genre verification")
	}
	device, e := os.ReadFile(filepath.Join(state, "device-id"))
	if e != nil {
		t.Fatal("device unavailable")
	}
	token, e := store.Load(string(device))
	if e != nil {
		t.Fatal("account unavailable")
	}
	a := music.New(token.Access)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	options, e := a.WaveOptions(ctx)
	if e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, o := range options {
		if o.Group == "genre" {
			count++
		}
	}
	if !hasWaveOption(options, "genre", "genre:rock") {
		t.Fatal("server rock genre unavailable")
	}
	var combined []music.WaveOption
	for _, o := range options {
		o.Group = "yandex/" + o.Group
		combined = append(combined, o)
	}
	z := zvuk.New("")
	zo, _ := z.WaveOptions(ctx)
	z.Close()
	for _, o := range zo {
		o.Group = "zvuk/" + o.Group
		combined = append(combined, o)
	}
	shared := sharedOptions(combined)
	common := 0
	for _, o := range shared {
		if o.Group == "shared/genre" {
			common++
		}
	}
	if common < 6 {
		t.Fatal("shared genre choices missing", common)
	}
	t.Logf("All mode: %d shared genre choices", common)
	w, e := a.WaveWithSeeds(ctx, "", nil, []string{"genre:rock"})
	if e != nil || len(w.Sequence) == 0 {
		t.Fatal(e)
	}
	t.Logf("%d genre choices; rock wave: %d tracks", count, len(w.Sequence))
}
