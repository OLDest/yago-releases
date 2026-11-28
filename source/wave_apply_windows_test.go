package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"yago/internal/music"
)

type applyRequest struct {
	provider, session string
	seeds             []string
}
type applyBackend struct {
	music.Backend
	provider string
	requests chan applyRequest
	streams  chan music.ID
	fail     bool
}

func (b *applyBackend) WaveWithSeeds(_ context.Context, session string, _, seeds []string) (music.Wave, error) {
	select {
	case b.requests <- applyRequest{b.provider, session, append([]string(nil), seeds...)}:
	default:
	}
	if b.fail {
		return music.Wave{}, errors.New("test wave unavailable")
	}
	prefix := ""
	if b.provider == "zvuk" {
		prefix = "zvuk-"
	}
	w := music.Wave{Session: prefix + "fresh", Batch: "new"}
	for _, id := range []string{"first", "second"} {
		w.Sequence = append(w.Sequence, struct {
			Track music.Track `json:"track"`
		}{Track: music.Track{ID: music.ID(prefix + id), DurationMS: 180000}})
	}
	return w, nil
}
func (b *applyBackend) Feedback(context.Context, string, string, string, music.Track, float64) error {
	return nil
}
func (b *applyBackend) Stream(_ context.Context, t music.Track, _ music.Quality) (music.StreamInfo, error) {
	select {
	case b.streams <- t.ID:
	default:
	}
	return music.StreamInfo{}, errors.New("test: no audio/network")
}
func TestApplyWaveClosesSettingsReloadsSeedsAndStartsFirstTrack(t *testing.T) {
	for _, mode := range []string{"yandex", "zvuk", "all"} {
		t.Run(mode, func(t *testing.T) {
			u := serviceTestUI(t)
			requests := make(chan applyRequest, 32)
			streams := make(chan music.ID, 4)
			u.services.yandex = &music.API{Backend: &applyBackend{provider: "yandex", requests: requests, streams: streams}}
			u.services.zvuk = &music.API{Backend: &applyBackend{provider: "zvuk", requests: requests, streams: streams}}
			u.services.mode = mode
			u.applyServiceMode(false)
			u.events = make(chan event, 32)
			u.disc.prefs.Seeds = map[string]string{"context": "activity:run"}
			u.disc.prefs.ZvukSeeds = map[string]string{"energy": "zvuk:energy:1"}
			u.pauseRequested = true
			u.stopped = true
			u.shuffled = true
			refillCancelled, refreshCancelled := false, false
			u.waveNextCancel = func() { refillCancelled = true }
			u.waveRefreshCancel = func() { refreshCancelled = true }
			u.waveRefreshing = true
			u.optionsMenu([]music.WaveOption{{Name: "Choice", Group: "language", Seed: "language:test"}})
			u.waveGroupMenu("language")
			u.applyWaveSettings()
			if u.disc.menu != "" || u.disc.waveOpenGroup != "" || !u.busy || u.section != sectionWave || !refillCancelled || !refreshCancelled {
				t.Fatal("apply did not close/cancel/navigate")
			}
			count := 1
			if mode == "all" {
				count = 2
			}
			for i := 0; i < count; i++ {
				select {
				case r := <-requests:
					if r.session != "" {
						t.Fatal("reused old session")
					}
					joined := strings.Join(r.seeds, ",")
					if r.provider == "yandex" && !strings.Contains(joined, "activity:run") || r.provider == "zvuk" && !strings.Contains(joined, "zvuk:energy:1") {
						t.Fatal("selected settings not sent", r.provider, joined)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("no wave request")
				}
			}
			select {
			case e := <-u.events:
				if e.kind != "wave" {
					t.Fatal(e.kind)
				}
				u.handle(e)
			case <-time.After(3 * time.Second):
				t.Fatal("no fresh wave")
			}
			if u.index != 0 || len(u.queue) == 0 || u.current.ID != u.queue[0].ID || u.pauseRequested || u.stopped || u.requestedOffset != 0 || u.order[0] != 0 {
				t.Fatal("first track not selected for immediate playback")
			}
			select {
			case id := <-streams:
				if id != u.queue[0].ID {
					t.Fatal("wrong stream started", id)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("no playback request")
			}
		})
	}
}
func TestApplyWaveWithoutAccountKeepsSettingsOpen(t *testing.T) {
	u := serviceTestUI(t)
	u.services.yandex = nil
	u.api = nil
	u.optionsMenu(nil)
	u.applyWaveSettings()
	if u.disc.menu != "wave-options" || u.busy {
		t.Fatal("closed settings without an account")
	}
}
