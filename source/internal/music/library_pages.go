package music

import (
	"context"
	"errors"
	"net/url"
)

type PartialLibraryError struct{ Err error }

func (e *PartialLibraryError) Error() string {
	return "Часть сервисов недоступна: " + e.Err.Error()
}
func (e *PartialLibraryError) Unwrap() error { return e.Err }
func IsPartialLibrary(err error) bool        { var p *PartialLibraryError; return errors.As(err, &p) }

func (a *API) LikesPage(ctx context.Context, uid ID, page int) ([]Track, bool, error) {
	if page < 0 || page > 1000 {
		return nil, false, errors.New("Неверная страница")
	}
	if b, ok := a.Backend.(interface {
		LikesPage(context.Context, ID, int) ([]Track, bool, error)
	}); ok {
		return b.LikesPage(ctx, uid, page)
	}
	if a.Backend != nil {
		ts, e := a.Likes(ctx, uid)
		start := min(page*PageSize, len(ts))
		end := min(start+PageSize, len(ts))
		return ts[start:end], end < len(ts), e
	}
	var data struct {
		Library struct {
			Tracks []struct {
				ID      ID `json:"id"`
				AlbumID ID `json:"albumId"`
			}
		}
	}
	e := a.callBounded(ctx, "GET", "/users/"+url.PathEscape(string(uid))+"/likes/tracks", nil, nil, &data, discoveryLimit)
	if e != nil {
		return nil, false, e
	}
	start := min(page*PageSize, len(data.Library.Tracks))
	end := min(start+PageSize, len(data.Library.Tracks))
	ids := make([]string, 0, end-start)
	for _, r := range data.Library.Tracks[start:end] {
		id := string(r.ID)
		if r.AlbumID != "" {
			id += ":" + string(r.AlbumID)
		}
		ids = append(ids, id)
	}
	ts, e := a.Tracks(ctx, ids)
	return ts, end < len(data.Library.Tracks), e
}

func interleave[T any](a, b []T) []T {
	out := make([]T, 0, len(a)+len(b))
	for i := 0; i < max(len(a), len(b)); i++ {
		if i < len(a) {
			out = append(out, a[i])
		}
		if i < len(b) {
			out = append(out, b[i])
		}
	}
	return out
}
func partialResult(success int, errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	err := errors.Join(errs...)
	if success > 0 {
		return &PartialLibraryError{Err: err}
	}
	return err
}

func (s *Services) LikesPage(ctx context.Context, _ ID, page int) ([]Track, bool, error) {
	var parts [2][]Track
	more, success := false, 0
	var errs []error
	for i, p := range []struct {
		a   *API
		uid ID
	}{{s.Yandex, s.YandexAccount.UID}, {s.Zvuk, s.ZvukAccount.UID}} {
		if p.a == nil {
			continue
		}
		ts, m, e := p.a.LikesPage(ctx, p.uid, page)
		if e != nil {
			errs = append(errs, e)
			continue
		}
		success++
		parts[i] = ts
		more = more || m
	}
	return interleave(parts[0], parts[1]), more, partialResult(success, errs)
}
