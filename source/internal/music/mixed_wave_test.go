package music

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type waveProbe struct {
	Backend
	prefix         string
	calls          int
	session, batch string
	feedbackID     ID
	seeds, queue   []string
	fail           bool
}

func (p *waveProbe) WaveWithSeeds(_ context.Context, session string, queue, seeds []string) (Wave, error) {
	p.calls++
	p.session = session
	p.seeds = append([]string(nil), seeds...)
	p.queue = append([]string(nil), queue...)
	if p.fail {
		return Wave{}, errors.New("unavailable")
	}
	w := Wave{Session: p.prefix + "session", Batch: fmt.Sprintf("%s%d", p.prefix, p.calls)}
	for i := 0; i < 5; i++ {
		w.Sequence = append(w.Sequence, struct {
			Track Track `json:"track"`
		}{Track: Track{ID: ID(fmt.Sprintf("%s%d", p.prefix, p.calls*10+i))}})
	}
	return w, nil
}
func (p *waveProbe) Feedback(_ context.Context, session, batch, _ string, t Track, _ float64) error {
	p.session = session
	p.batch = batch
	p.feedbackID = t.ID
	return nil
}
func (p *waveProbe) WaveOptions(context.Context) ([]WaveOption, error) {
	return []WaveOption{{Name: "Language", Seed: p.prefix + "lang", Group: "language"}}, nil
}
func TestMixedWaveRoutingContinuationAndIsolation(t *testing.T) {
	y, z := &waveProbe{}, &waveProbe{prefix: "zvuk-"}
	s := &Services{Yandex: &API{Backend: y}, Zvuk: &API{Backend: z}}
	ctx := context.Background()
	w, e := s.WaveWithSeeds(ctx, "", []string{"1", "zvuk-1"}, []string{"language:russian", "zvuk:language:russian"})
	if e != nil || len(w.Sequence) != 10 {
		t.Fatal("mixed batch", e, len(w.Sequence))
	}
	for i, item := range w.Sequence {
		if i > 0 && IsZvuk(item.Track.ID) == IsZvuk(w.Sequence[i-1].Track.ID) {
			t.Fatal("not interleaved")
		}
	}
	if len(y.seeds) != 1 || y.seeds[0] != "language:russian" || len(z.seeds) != 1 || z.seeds[0] != "zvuk:language:russian" || len(y.queue) != 1 || y.queue[0] != "1" || len(z.queue) != 1 || z.queue[0] != "zvuk-1" {
		t.Fatal("foreign settings/IDs sent to provider")
	}
	state, e := decodeMixedWave(w.Session)
	if e != nil || state.Yandex.Session != "session" || state.Zvuk.Session != "zvuk-session" {
		t.Fatal(state, e)
	}
	if _, e = s.WaveWithSeeds(ctx, w.Session, nil, nil); e != nil || y.session != "session" || z.session != "zvuk-session" {
		t.Fatal("lost per-provider continuation", e)
	}
	for _, id := range []ID{"1", "zvuk-1"} {
		if e = s.Feedback(ctx, w.Session, w.Batch, "skip", Track{ID: id}, 20); e != nil {
			t.Fatal(e)
		}
	}
	if y.session != "session" || y.batch != "1" || y.feedbackID != "1" || z.session != "zvuk-session" || z.batch != "zvuk-1" || z.feedbackID != "zvuk-1" {
		t.Fatal("feedback crossed provider/batch")
	}
	opts, e := s.WaveOptions(ctx)
	if e != nil || len(opts) != 2 || opts[0].Group != "yandex/language" || opts[1].Group != "zvuk/language" {
		t.Fatal(opts, e)
	}
}
func TestMixedWaveWorksWithOneUnavailableProvider(t *testing.T) {
	y, z := &waveProbe{fail: true}, &waveProbe{prefix: "zvuk-"}
	s := &Services{Yandex: &API{Backend: y}, Zvuk: &API{Backend: z}}
	w, e := s.WaveWithSeeds(context.Background(), "", nil, nil)
	if e != nil || len(w.Sequence) != 5 {
		t.Fatal(e)
	}
	for _, item := range w.Sequence {
		if !IsZvuk(item.Track.ID) {
			t.Fatal("foreign track")
		}
	}
	z.fail = true
	if _, e = s.WaveWithSeeds(context.Background(), w.Session, nil, nil); e == nil {
		t.Fatal("both failed")
	}
	if _, e = s.WaveWithSeeds(context.Background(), mixedWavePrefix+strings.Repeat("x", 5000), nil, nil); e == nil {
		t.Fatal("unbounded session accepted")
	}
}
