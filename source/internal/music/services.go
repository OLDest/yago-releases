package music

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
)

// Services routes each ID to its own credentials. All combines metadata, never audio buffers.
type Services struct {
	Yandex, Zvuk               *API
	Musify                     *API
	YandexAccount, ZvukAccount Account
}

var _ Backend = (*Services)(nil)

func IsZvuk(id ID) bool { return strings.HasPrefix(string(id), "zvuk-") }
func Source(id ID) string {
	if IsMusify(id) {
		return "Musify"
	}
	if IsZvuk(id) {
		return "Звук"
	}
	return "Яндекс"
}
func (s *Services) target(id ID) (*API, ID, error) {
	if IsMusify(id) {
		if s.Musify != nil {
			return s.Musify, "musify-guest", nil
		}
		return nil, "", errors.New("Войдите в Musify для этого трека")
	}
	if IsZvuk(id) {
		if s.Zvuk != nil {
			return s.Zvuk, s.ZvukAccount.UID, nil
		}
	} else if s.Yandex != nil {
		return s.Yandex, s.YandexAccount.UID, nil
	}
	return nil, "", fmt.Errorf("Войдите в %s для этого трека", Source(id))
}
func (s *Services) Account(context.Context) (Account, error) {
	if s.Yandex == nil && s.Zvuk == nil && s.Musify != nil {
		return Account{UID: "all-musify", DisplayName: "Все сервисы"}, nil
	}
	if s.Yandex == nil && s.Zvuk == nil {
		return Account{}, errors.New("Подключите музыкальный сервис")
	}
	sum := sha256.Sum256([]byte(string(s.YandexAccount.UID) + "\x00" + string(s.ZvukAccount.UID)))
	return Account{UID: ID(fmt.Sprintf("all-%x", sum[:12])), DisplayName: "Все сервисы"}, nil
}
func (s *Services) Search(ctx context.Context, q string) ([]Track, error) {
	r, e := s.SearchPage(ctx, q, "track", 0)
	return r.Tracks, e
}
func (s *Services) SearchPage(ctx context.Context, q, kind string, page int) (SearchPage, error) {
	var out SearchPage
	var errs []error
	success := 0
	for _, a := range []*API{s.Yandex, s.Zvuk, s.Musify} {
		if a == nil {
			continue
		}
		r, e := a.SearchPage(ctx, q, kind, page)
		if e != nil {
			errs = append(errs, e)
			continue
		}
		success++
		out.Tracks = append(out.Tracks, r.Tracks...)
		out.Albums = append(out.Albums, r.Albums...)
		out.Artists = append(out.Artists, r.Artists...)
		out.Playlists = append(out.Playlists, r.Playlists...)
		out.More = out.More || r.More
	}
	if len(errs) > 0 {
		if success == 0 {
			return out, errors.Join(errs...)
		}
		out.Warning = "Часть сервисов недоступна: " + errors.Join(errs...).Error()
	}
	return out, nil
}
func (s *Services) Tracks(ctx context.Context, ids []string) ([]Track, error) {
	// Preserve requested order while batching each provider once.
	var y, z, mus []string
	for _, id := range ids {
		if IsMusify(ID(id)) {
			if s.Musify != nil {
				mus = append(mus, id)
			}
			continue
		}
		if IsZvuk(ID(id)) {
			z = append(z, id)
		} else {
			y = append(y, id)
		}
	}
	m := make(map[ID]Track, len(ids))
	for _, batch := range [][]string{y, z, mus} {
		if len(batch) == 0 {
			continue
		}
		a, _, e := s.target(ID(batch[0]))
		if e != nil {
			return nil, e
		}
		ts, e := a.Tracks(ctx, batch)
		if e != nil {
			return nil, e
		}
		for _, t := range ts {
			m[t.ID] = t
		}
	}
	out := make([]Track, 0, len(ids))
	for _, id := range ids {
		key := strings.SplitN(id, ":", 2)[0]
		if t, ok := m[ID(key)]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}
func (s *Services) Likes(ctx context.Context, _ ID) ([]Track, error) {
	var parts [2][]Track
	var errs []error
	success := 0
	limit := 2000
	if s.Yandex != nil && s.Zvuk != nil {
		limit = 1000
	}
	for i, pair := range []struct {
		a   *API
		uid ID
	}{{s.Yandex, s.YandexAccount.UID}, {s.Zvuk, s.ZvukAccount.UID}} {
		if pair.a == nil {
			continue
		}
		ts, e := pair.a.Likes(ctx, pair.uid)
		if e != nil {
			errs = append(errs, e)
			continue
		}
		success++
		parts[i] = ts[:min(len(ts), limit)]
	}
	return interleave(parts[0], parts[1]), partialResult(success, errs)
}
func (s *Services) Playlists(ctx context.Context, _ ID) ([]Playlist, error) {
	var parts [3][]Playlist
	var errs []error
	success := 0
	for i, pair := range []struct {
		a   *API
		uid ID
	}{{s.Yandex, s.YandexAccount.UID}, {s.Zvuk, s.ZvukAccount.UID}, {s.Musify, "musify-guest"}} {
		if pair.a == nil {
			continue
		}
		ps, e := pair.a.Playlists(ctx, pair.uid)
		if e != nil {
			errs = append(errs, e)
			continue
		}
		success++
		parts[i] = ps
	}
	return interleave(interleave(parts[0], parts[1]), parts[2]), partialResult(success, errs)
}
func (s *Services) Playlist(ctx context.Context, uid, id ID) ([]Track, error) {
	a, owner, e := s.target(id)
	if e != nil {
		return nil, e
	}
	if uid == "" || strings.HasPrefix(string(uid), "all-") {
		uid = owner
	}
	return a.Playlist(ctx, uid, id)
}
func (s *Services) Like(ctx context.Context, _ ID, t Track) error {
	a, uid, e := s.target(t.ID)
	if e != nil {
		return e
	}
	return a.Like(ctx, uid, t)
}
func (s *Services) Unlike(ctx context.Context, _ ID, t Track) error {
	a, uid, e := s.target(t.ID)
	if e != nil {
		return e
	}
	return a.Unlike(ctx, uid, t)
}
func (s *Services) Dislike(ctx context.Context, _ ID, kind string, id ID, remove bool) error {
	a, uid, e := s.target(id)
	if e != nil {
		return e
	}
	return a.Dislike(ctx, uid, kind, id, remove)
}
func (s *Services) Stream(ctx context.Context, t Track, q Quality) (StreamInfo, error) {
	a, _, e := s.target(t.ID)
	if e != nil {
		return StreamInfo{}, e
	}
	return a.Stream(ctx, t, q)
}
func (s *Services) AlbumPage(ctx context.Context, id ID, page int) ([]Track, bool, error) {
	a, _, e := s.target(id)
	if e != nil {
		return nil, false, e
	}
	return a.AlbumPage(ctx, id, page)
}
func (s *Services) ArtistTracks(ctx context.Context, id ID, page int) ([]Track, bool, error) {
	a, _, e := s.target(id)
	if e != nil {
		return nil, false, e
	}
	return a.ArtistTracks(ctx, id, page)
}
func (s *Services) History(ctx context.Context) ([]HistoryRef, error) {
	var parts [3][]HistoryRef
	var errs []error
	success := 0
	for i, a := range []*API{s.Yandex, s.Zvuk, s.Musify} {
		if a == nil {
			continue
		}
		refs, e := a.History(ctx)
		if e != nil {
			errs = append(errs, e)
			continue
		}
		success++
		parts[i] = refs
	}
	return MergeHistory(MergeHistory(parts[0], parts[1], 400), parts[2], 400), partialResult(success, errs)
}
func (s *Services) HistoryTracks(ctx context.Context, refs []HistoryRef, page int) ([]Track, bool, error) {
	if page < 0 || page > 1000 {
		return nil, false, errors.New("Некорректная страница")
	}
	start := min(page*PageSize, len(refs))
	end := min(start+PageSize, len(refs))
	ids := make([]string, 0, end-start)
	for _, r := range refs[start:end] {
		id := string(r.ID)
		if r.AlbumID != "" {
			id += ":" + string(r.AlbumID)
		}
		ids = append(ids, id)
	}
	ts, e := s.Tracks(ctx, ids)
	return ts, end < len(refs), e
}
func (s *Services) WaveWithSeeds(ctx context.Context, session string, queue, seeds []string) (Wave, error) {
	return s.mixedWave(ctx, session, queue, seeds)
}
func (s *Services) Feedback(ctx context.Context, session, batch, kind string, t Track, played float64) error {
	if IsMusify(t.ID) {
		return nil
	}
	if IsMixedWave(session) {
		return s.mixedFeedback(ctx, session, kind, t, played)
	}
	if IsZvuk(t.ID) || strings.HasPrefix(session, "zvuk-wave:") {
		if s.Zvuk != nil {
			return s.Zvuk.Feedback(ctx, session, batch, kind, t, played)
		}
		return nil
	}
	if s.Yandex == nil {
		return nil
	}
	return s.Yandex.Feedback(ctx, session, batch, kind, t, played)
}
func (s *Services) WaveOptions(ctx context.Context) ([]WaveOption, error) {
	return s.mixedWaveOptions(ctx)
}
func (s *Services) Suggest(ctx context.Context, q string) ([]string, error) {
	if s.Yandex != nil {
		return s.Yandex.Suggest(ctx, q)
	}
	if s.Zvuk != nil {
		return s.Zvuk.Suggest(ctx, q)
	}
	return nil, nil
}
