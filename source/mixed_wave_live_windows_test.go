package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"yago/internal/music"
	"yago/internal/store"
	"yago/internal/zvuk"
)

func TestLiveMixedWave(t *testing.T) {
	state := os.Getenv("YAGO_LIVE_WAVE_STATE")
	if state == "" {
		t.Skip("explicit own account check")
	}
	device, e := os.ReadFile(filepath.Join(state, "device-id"))
	if e != nil || len(device) != 32 {
		t.Fatal("device unavailable")
	}
	yt, e := store.Load(string(device))
	if e != nil {
		t.Fatal("saved Yandex unavailable")
	}
	zt, e := store.Load(string(device) + ":zvuk")
	if e != nil {
		t.Fatal("saved Zvuk unavailable")
	}
	z := zvuk.New(zt.Access)
	defer z.Close()
	s := &music.Services{Yandex: music.New(yt.Access), Zvuk: &music.API{Backend: z}}
	api := &music.API{Backend: s}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	options, e := api.WaveOptions(ctx)
	if e != nil {
		t.Fatal(e)
	}
	yc, zc := 0, 0
	for _, o := range options {
		if o.Group == "yandex/language" || o.Group == "yandex/moodEnergy" {
			t.Logf("shared mapping option: %s | %s | %s", o.Group, o.Name, o.Seed)
		}
		if strings.HasPrefix(o.Group, "zvuk/") {
			zc++
		} else {
			yc++
		}
	}
	if yc == 0 || zc == 0 {
		t.Fatal("settings missing provider", yc, zc)
	}
	shared := sharedOptions(options)
	nshared := 0
	for _, o := range shared {
		if strings.HasPrefix(o.Group, "shared/") {
			nshared++
		}
	}
	if nshared != 7 {
		t.Fatal("shared mappings unavailable", nshared)
	}
	u := &UI{services: servicesUI{enabled: true, mode: "all"}}
	u.disc.prefs.Seeds = map[string]string{}
	u.disc.prefs.ZvukSeeds = map[string]string{}
	for _, c := range sharedWaveChoices {
		if c.Seed == "shared:language:russian" || c.Seed == "shared:mood:calm" {
			u.disc.prefs.Seeds[c.YGroup] = c.YSeed
			u.disc.prefs.ZvukSeeds[c.ZGroup] = c.ZSeed
			if c.ZGroup2 != "" {
				u.disc.prefs.ZvukSeeds[c.ZGroup2] = c.ZSeed2
			}
		}
	}
	seeds := u.waveSeeds()
	if probe, e := z.WaveWithSeeds(ctx, "", nil, seeds); e != nil {
		t.Fatal("Zvuk shared request:", e)
	} else {
		t.Logf("Zvuk shared request: %d tracks", len(probe.Sequence))
	}
	w, e := api.WaveWithSeeds(ctx, "", nil, seeds)
	if e != nil {
		t.Fatal(e)
	}
	ids := []string{}
	ycount, zcount := 0, 0
	for _, v := range w.Sequence {
		ids = append(ids, string(v.Track.ID))
		if music.IsZvuk(v.Track.ID) {
			zcount++
		} else {
			ycount++
		}
	}
	if ycount == 0 || zcount == 0 {
		t.Fatal("missing source", ycount, zcount)
	}
	tracks, contexts, e := fetchWaveTail(ctx, api, w.Session, ids, seeds, store.Preferences{})
	if e != nil || len(tracks) != 15 || len(contexts) != 15 {
		t.Fatal("mixed refill", len(tracks), e)
	}
	ytail, ztail := 0, 0
	for i, tr := range tracks {
		if music.IsZvuk(tr.ID) {
			ztail++
		} else {
			ytail++
		}
		if !music.IsMixedWave(contexts[i].Session) {
			t.Fatal("lost routing context")
		}
	}
	if ytail == 0 || ztail == 0 {
		t.Fatal("refill missing provider")
	}
	t.Logf("settings Yandex=%d Zvuk=%d; initial tracks Yandex=%d Zvuk=%d; 15-track refill Yandex=%d Zvuk=%d", yc, zc, ycount, zcount, ytail, ztail)
}
