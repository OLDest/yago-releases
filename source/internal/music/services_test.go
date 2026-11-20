package music

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type routeProbe struct {
	Backend
	calls  int
	uid    ID
	id     ID
	fail   bool
	tracks []Track
	refs   []HistoryRef
	lists  []Playlist
}

func (p *routeProbe) Likes(context.Context, ID) ([]Track, error) {
	if p.fail {
		return nil, errors.New("unavailable")
	}
	return p.tracks, nil
}
func (p *routeProbe) LikesPage(_ context.Context, _ ID, page int) ([]Track, bool, error) {
	if p.fail {
		return nil, false, errors.New("unavailable")
	}
	start := min(page*PageSize, len(p.tracks))
	end := min(start+PageSize, len(p.tracks))
	return p.tracks[start:end], end < len(p.tracks), nil
}
func (p *routeProbe) History(context.Context) ([]HistoryRef, error) {
	if p.fail {
		return nil, errors.New("unavailable")
	}
	return p.refs, nil
}
func (p *routeProbe) Playlists(context.Context, ID) ([]Playlist, error) {
	if p.fail {
		return nil, errors.New("unavailable")
	}
	return p.lists, nil
}

func (p *routeProbe) Like(_ context.Context, uid ID, t Track) error {
	p.calls++
	p.uid = uid
	p.id = t.ID
	return nil
}
func (p *routeProbe) Stream(_ context.Context, t Track, _ Quality) (StreamInfo, error) {
	p.calls++
	p.id = t.ID
	return StreamInfo{URL: string(t.ID)}, nil
}
func (p *routeProbe) Tracks(_ context.Context, ids []string) ([]Track, error) {
	p.calls++
	ts := []Track{}
	for _, id := range ids {
		ts = append(ts, Track{ID: ID(id)})
	}
	return ts, nil
}
func (p *routeProbe) SearchPage(context.Context, string, string, int) (SearchPage, error) {
	if p.fail {
		return SearchPage{}, errors.New("provider unavailable")
	}
	return SearchPage{Tracks: p.tracks}, nil
}
func TestServicesRouteCredentialsAndKeepIdenticalIDsSeparate(t *testing.T) {
	ctx := context.Background()
	y, z := &routeProbe{}, &routeProbe{}
	s := &Services{Yandex: &API{Backend: y}, Zvuk: &API{Backend: z}, YandexAccount: Account{UID: "y-user"}, ZvukAccount: Account{UID: "zvuk-user"}}
	for _, id := range []ID{"123", "zvuk-123"} {
		if e := s.Like(ctx, "combined-user", Track{ID: id}); e != nil {
			t.Fatal(e)
		}
	}
	if y.calls != 1 || z.calls != 1 || y.uid != "y-user" || z.uid != "zvuk-user" {
		t.Fatal("credential routing", y, z)
	}
	ts, e := s.Tracks(ctx, []string{"zvuk-123", "123", "zvuk-123"})
	if e != nil || len(ts) != 3 || ts[0].ID != "zvuk-123" || ts[1].ID != "123" {
		t.Fatal(ts, e)
	}
	s.Zvuk = nil
	if _, e = s.Stream(ctx, Track{ID: "zvuk-123"}, High); e == nil || y.calls != 2 {
		t.Fatal("missing provider fell back", y.calls, e)
	}
}
func TestAllSearchRetainsWorkingServiceAndWarns(t *testing.T) {
	y, z := &routeProbe{tracks: []Track{{ID: "123"}}}, &routeProbe{fail: true}
	s := &Services{Yandex: &API{Backend: y}, Zvuk: &API{Backend: z}}
	r, e := s.SearchPage(context.Background(), "q", "all", 0)
	if e != nil || len(r.Tracks) != 1 || r.Warning == "" {
		t.Fatal(r, e)
	}
	y.fail = true
	if _, e = s.SearchPage(context.Background(), "q", "all", 0); e == nil {
		t.Fatal("all errors hidden")
	}
}
func TestCombinedLibraryShowsBothSourcesOnFirstPage(t *testing.T) {
	y, z := &routeProbe{}, &routeProbe{}
	for i := 0; i < 80; i++ {
		id := ID(fmt.Sprint(i))
		y.tracks = append(y.tracks, Track{ID: id})
		y.refs = append(y.refs, HistoryRef{ID: id, Date: "2026-11-17"})
	}
	z.tracks = []Track{{ID: "zvuk-1"}, {ID: "zvuk-2"}}
	z.refs = []HistoryRef{{ID: "zvuk-1", Date: "2026-11-17T12:00:00Z"}}
	y.lists = []Playlist{{Kind: "1"}, {Kind: "2"}}
	z.lists = []Playlist{{Kind: "zvuk-1"}}
	s := &Services{Yandex: &API{Backend: y}, Zvuk: &API{Backend: z}}
	ts, more, e := s.LikesPage(context.Background(), "all", 0)
	if e != nil || !more || ts[1].ID != "zvuk-1" || len(ts) != 42 {
		t.Fatal("favorites merge", e)
	}
	refs, e := s.History(context.Background())
	if e != nil || refs[1].ID != "zvuk-1" {
		t.Fatal("history merge", e)
	}
	lists, e := s.Playlists(context.Background(), "all")
	if e != nil || lists[1].Kind != "zvuk-1" {
		t.Fatal("playlist merge", e)
	}
	z.fail = true
	ts, _, e = s.LikesPage(context.Background(), "all", 0)
	if !IsPartialLibrary(e) || len(ts) != 40 {
		t.Fatal("partial favorites discarded", e)
	}
	lists, e = s.Playlists(context.Background(), "all")
	if !IsPartialLibrary(e) || len(lists) != 2 {
		t.Fatal("partial playlists discarded", e)
	}
}
