package music

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"strings"
	"sync"
)

const mixedWavePrefix = "mixed-wave:"
const ExcludeZvukWave = "service:exclude:zvuk"

type mixedWaveState struct{ Yandex, Zvuk WaveContext }

func IsMixedWave(session string) bool { return strings.HasPrefix(session, mixedWavePrefix) }
func decodeMixedWave(session string) (mixedWaveState, error) {
	var state mixedWaveState
	if !IsMixedWave(session) {
		return state, nil
	}
	if len(session) > 4096 {
		return state, errors.New("Слишком большая сессия общей волны")
	}
	err := json.Unmarshal([]byte(strings.TrimPrefix(session, mixedWavePrefix)), &state)
	return state, err
}
func (s *Services) mixedWave(ctx context.Context, session string, queue, seeds []string) (Wave, error) {
	state, err := decodeMixedWave(session)
	if err != nil {
		return Wave{}, err
	}
	// A restored queue from an older build can have a single-provider session.
	if !IsMixedWave(session) && session != "" {
		if strings.HasPrefix(session, "zvuk-wave:") || strings.HasPrefix(session, "zvuk-radio:") {
			state.Zvuk.Session = session
		} else {
			state.Yandex.Session = session
		}
	}
	apis := [2]*API{s.Yandex, s.Zvuk}
	for _, seed := range seeds {
		if seed == ExcludeZvukWave {
			apis[1] = nil
		}
	}
	for _, seed := range seeds {
		if strings.HasPrefix(seed, "zvuk:radio:") {
			apis[0] = nil
			break
		}
		if strings.HasPrefix(seed, "track:") || strings.HasPrefix(seed, "artist:") || strings.HasPrefix(seed, "album:") || strings.HasPrefix(seed, "playlist:") {
			apis[1] = nil
			break
		}
	}
	previous := [2]WaveContext{state.Yandex, state.Zvuk}
	var result [2]Wave
	var errs [2]error
	var wg sync.WaitGroup
	for i, api := range apis {
		if api == nil {
			continue
		}
		wg.Add(1)
		go func(i int, api *API) {
			defer wg.Done()
			ownSeeds, ownQueue := []string{}, []string{}
			for _, seed := range seeds {
				if seed == ExcludeZvukWave {
					continue
				}
				if strings.HasPrefix(seed, "zvuk:") == (i == 1) {
					ownSeeds = append(ownSeeds, seed)
				}
			}
			for _, id := range queue {
				if IsZvuk(ID(id)) == (i == 1) {
					ownQueue = append(ownQueue, id)
				}
			}
			result[i], errs[i] = api.WaveWithSeeds(ctx, previous[i].Session, ownQueue, ownSeeds)
			if errs[i] != nil && previous[i].Session != "" && ctx.Err() == nil {
				result[i], errs[i] = api.WaveWithSeeds(ctx, "", ownQueue, ownSeeds)
			}
		}(i, api)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return Wave{}, ctx.Err()
	}
	for i, v := range result {
		if errs[i] == nil && v.Session != "" {
			previous[i] = WaveContext{Session: v.Session, Batch: v.Batch}
		}
	}
	state = mixedWaveState{Yandex: previous[0], Zvuk: previous[1]}
	raw, _ := json.Marshal(state)
	if len(raw)+len(mixedWavePrefix) > 4096 {
		return Wave{}, errors.New("Слишком большая сессия общей волны")
	}
	out := Wave{Session: mixedWavePrefix + string(raw), Batch: "mixed"}
	// Alternate sources with a random starting side. Only metadata is retained.
	first := rand.IntN(2)
	seen := make(map[ID]bool, len(queue)+10)
	for _, id := range queue {
		seen[ID(id)] = true
	}
	for p := 0; p < max(len(result[0].Sequence), len(result[1].Sequence)); p++ {
		for n := 0; n < 2; n++ {
			i := (first + n) % 2
			if errs[i] != nil || p >= len(result[i].Sequence) {
				continue
			}
			item := result[i].Sequence[p]
			if item.Track.ID != "" && !seen[item.Track.ID] && IsZvuk(item.Track.ID) == (i == 1) {
				out.Sequence = append(out.Sequence, item)
				seen[item.Track.ID] = true
			}
		}
	}
	if len(out.Sequence) == 0 {
		return Wave{}, errors.Join(errors.New("Общая волна не вернула новые треки"), errs[0], errs[1])
	}
	return out, nil
}

func (s *Services) mixedFeedback(ctx context.Context, session, kind string, t Track, played float64) error {
	state, err := decodeMixedWave(session)
	if err != nil {
		return err
	}
	if t.ID == "" {
		if kind != "radioStarted" {
			return nil
		}
		var errs []error
		for i, api := range []*API{s.Yandex, s.Zvuk} {
			c := state.Yandex
			if i == 1 {
				c = state.Zvuk
			}
			if api != nil && c.Session != "" {
				errs = append(errs, api.Feedback(ctx, c.Session, c.Batch, kind, t, played))
			}
		}
		return errors.Join(errs...)
	}
	api, c := s.Yandex, state.Yandex
	if IsZvuk(t.ID) {
		api, c = s.Zvuk, state.Zvuk
	}
	if api == nil || c.Session == "" {
		return nil
	}
	return api.Feedback(ctx, c.Session, c.Batch, kind, t, played)
}
func (s *Services) mixedWaveOptions(ctx context.Context) ([]WaveOption, error) {
	var out []WaveOption
	var errs []error
	for i, api := range []*API{s.Yandex, s.Zvuk} {
		if api == nil {
			continue
		}
		options, err := api.WaveOptions(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		prefix := "yandex/"
		if i == 1 {
			prefix = "zvuk/"
		}
		for _, o := range options {
			o.Group = prefix + o.Group
			out = append(out, o)
		}
	}
	if len(out) == 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}
