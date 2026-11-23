package music

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const PageSize = 40
const discoveryLimit = 2 << 20

// Decode only a page of models. Large arrays never retain thousands of objects.
type pageItems[T any] []T

func (p *pageItems[T]) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		*p = nil
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(b))
	tok, e := d.Token()
	if e != nil {
		return e
	}
	if tok != json.Delim('[') {
		return errors.New("ожидался список")
	}
	*p = nil
	var discard json.RawMessage
	for d.More() {
		if len(*p) < PageSize {
			var v T
			if e = d.Decode(&v); e != nil {
				return e
			}
			*p = append(*p, v)
		} else if e = d.Decode(&discard); e != nil {
			return e
		}
	}
	_, e = d.Token()
	return e
}

type SearchPage struct {
	Tracks    []Track
	Albums    []Album
	Artists   []Named
	Playlists []Playlist
	More      bool
	Warning   string
}
type searchGroup[T any] struct {
	Results pageItems[T] `json:"results"`
	Total   int          `json:"total"`
	PerPage int          `json:"perPage"`
}

func groupMore[T any](g searchGroup[T], page int) bool {
	size := g.PerPage
	if size <= 0 {
		size = len(g.Results)
	}
	return size > 0 && (page+1)*size < g.Total
}
func (a *API) SearchPage(ctx context.Context, q, kind string, page int) (SearchPage, error) {
	if a.Backend != nil {
		return a.Backend.SearchPage(ctx, q, kind, page)
	}
	if kind != "all" && kind != "track" && kind != "album" && kind != "artist" && kind != "playlist" {
		return SearchPage{}, errors.New("неизвестный тип поиска")
	}
	if page < 0 || page > 1000 || len(q) > 1000 {
		return SearchPage{}, errors.New("неверный запрос")
	}
	var v struct {
		Tracks    searchGroup[Track]    `json:"tracks"`
		Albums    searchGroup[Album]    `json:"albums"`
		Artists   searchGroup[Named]    `json:"artists"`
		Playlists searchGroup[Playlist] `json:"playlists"`
	}
	query := url.Values{"text": {q}, "type": {kind}, "page": {strconv.Itoa(page)}, "page-size": {strconv.Itoa(PageSize)}}
	e := a.callBounded(ctx, "GET", "/search?"+query.Encode(), nil, nil, &v, discoveryLimit)
	return SearchPage{Tracks: v.Tracks.Results, Albums: v.Albums.Results, Artists: v.Artists.Results, Playlists: v.Playlists.Results, More: groupMore(v.Tracks, page) || groupMore(v.Albums, page) || groupMore(v.Artists, page) || groupMore(v.Playlists, page)}, e
}
func (a *API) Suggest(ctx context.Context, q string) ([]string, error) {
	if a.Backend != nil {
		return a.Backend.Suggest(ctx, q)
	}
	var v struct {
		Suggestions pageItems[string] `json:"suggestions"`
	}
	e := a.callBounded(ctx, "GET", "/search/suggest?part="+url.QueryEscape(q), nil, nil, &v, 256<<10)
	if len(v.Suggestions) > 5 {
		v.Suggestions = v.Suggestions[:5]
	}
	return v.Suggestions, e
}

type albumWindow struct {
	Offset, Total int
	Tracks        []Track
}

func (w *albumWindow) UnmarshalJSON(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	tok, e := d.Token()
	if e != nil {
		return e
	}
	if tok != json.Delim('[') {
		return errors.New("неверный список дисков")
	}
	var discard json.RawMessage
	for d.More() {
		tok, e = d.Token()
		if e != nil || tok != json.Delim('[') {
			return errors.New("неверный список треков")
		}
		for d.More() {
			if w.Total >= w.Offset && len(w.Tracks) < PageSize {
				var t Track
				if e = d.Decode(&t); e != nil {
					return e
				}
				if t.ID != "" {
					w.Tracks = append(w.Tracks, t)
				}
			} else if e = d.Decode(&discard); e != nil {
				return e
			}
			w.Total++
		}
		if _, e = d.Token(); e != nil {
			return e
		}
	}
	_, e = d.Token()
	return e
}
func (a *API) AlbumPage(ctx context.Context, id ID, page int) ([]Track, bool, error) {
	if a.Backend == nil && IsZvuk(id) {
		return nil, false, errors.New("Объект принадлежит Звуку")
	}
	if a.Backend != nil {
		return a.Backend.AlbumPage(ctx, id, page)
	}
	if page < 0 || page > 1000 {
		return nil, false, errors.New("неверная страница")
	}
	v := struct {
		Volumes albumWindow `json:"volumes"`
	}{Volumes: albumWindow{Offset: page * PageSize}}
	e := a.callBounded(ctx, "GET", "/albums/"+url.PathEscape(string(id))+"/with-tracks", nil, nil, &v, discoveryLimit)
	return v.Volumes.Tracks, (page+1)*PageSize < v.Volumes.Total, e
}
func (a *API) ArtistTracks(ctx context.Context, id ID, page int) ([]Track, bool, error) {
	if a.Backend == nil && IsZvuk(id) {
		return nil, false, errors.New("Объект принадлежит Звуку")
	}
	if a.Backend != nil {
		return a.Backend.ArtistTracks(ctx, id, page)
	}
	var v struct {
		Tracks pageItems[Track]             `json:"tracks"`
		Pager  struct{ Total, PerPage int } `json:"pager"`
	}
	e := a.callBounded(ctx, "GET", fmt.Sprintf("/artists/%s/tracks?page=%d&page-size=%d", url.PathEscape(string(id)), page, PageSize), nil, nil, &v, discoveryLimit)
	return v.Tracks, len(v.Tracks) > 0 && (page+1)*PageSize < v.Pager.Total, e
}
func (a *API) Unlike(ctx context.Context, uid ID, t Track) error {
	if a.Backend == nil && IsZvuk(t.ID) {
		return errors.New("Объект принадлежит Звуку. Подключите этот сервис")
	}
	if a.Backend != nil {
		return a.Backend.Unlike(ctx, uid, t)
	}
	return a.callBounded(ctx, "POST", "/users/"+url.PathEscape(string(uid))+"/likes/tracks/remove", url.Values{"track-ids": {t.FullID()}}, nil, nil, 256<<10)
}
func (a *API) Dislike(ctx context.Context, uid ID, kind string, id ID, remove bool) error {
	if a.Backend == nil && IsZvuk(id) {
		return errors.New("Объект принадлежит Звуку. Подключите этот сервис")
	}
	if a.Backend != nil {
		return a.Backend.Dislike(ctx, uid, kind, id, remove)
	}
	if kind != "track" && kind != "artist" {
		return errors.New("неверный тип блокировки")
	}
	action := "add-multiple"
	if remove {
		action = "remove"
	}
	return a.callBounded(ctx, "POST", "/users/"+url.PathEscape(string(uid))+"/dislikes/"+kind+"s/"+action, url.Values{kind + "-ids": {string(id)}}, nil, nil, 256<<10)
}

type WaveOption struct{ Name, Seed, Group string }

func (a *API) WaveOptions(ctx context.Context) ([]WaveOption, error) {
	if a.Backend != nil {
		return a.Backend.WaveOptions(ctx)
	}
	type value struct {
		Name string `json:"name"`
		Seed string `json:"serializedSeed"`
	}
	var v struct {
		Blocks pageItems[struct {
			Items pageItems[struct {
				Name string                     `json:"name"`
				ID   struct{ Type, Tag string } `json:"id"`
			}] `json:"items"`
		}] `json:"blocks"`
		Restrictions map[string]struct {
			Name           string           `json:"name"`
			PossibleValues pageItems[value] `json:"possibleValues"`
			Min            value            `json:"min"`
			Max            value            `json:"max"`
		} `json:"settingRestrictions"`
	}
	e := a.callBounded(ctx, "GET", "/rotor/wave/settings?seeds=user%3Aonyourwave", nil, nil, &v, discoveryLimit)
	if e != nil {
		return nil, e
	}
	out := []WaveOption{{Name: "Моя волна", Seed: "user:onyourwave", Group: "context"}}
	for _, block := range v.Blocks {
		for _, item := range block.Items {
			if item.ID.Type != "" && item.ID.Tag != "" && len(out) < 80 {
				out = append(out, WaveOption{item.Name, item.ID.Type + ":" + item.ID.Tag, "context"})
			}
		}
	}
	// Stable order; no assumptions about which values the server enables.
	for _, key := range []string{"diversity", "moodEnergy", "language", "mood", "energy"} {
		r := v.Restrictions[key]
		if len(r.PossibleValues) == 0 {
			r.PossibleValues = append(r.PossibleValues, r.Min, r.Max)
		}
		for _, x := range r.PossibleValues {
			if x.Seed != "" && strings.Contains(x.Seed, ":") && len(out) < 80 {
				name := x.Name
				if r.Name != "" {
					name = r.Name + ": " + name
				}
				out = append(out, WaveOption{name, x.Seed, key})
			}
		}
	}
	out = append(out, a.genreWaveOptions(ctx)...)
	return out, nil
}

type historyItem struct {
	Type string `json:"type"`
	Data struct {
		ID struct {
			TrackID ID `json:"trackId"`
			AlbumID ID `json:"albumId"`
		} `json:"itemId"`
	} `json:"data"`
}
type HistoryRef struct {
	ID, AlbumID ID
	Date        string
}

func (a *API) History(ctx context.Context) ([]HistoryRef, error) {
	if a.Backend != nil {
		return a.Backend.History(ctx)
	}
	var v struct {
		Tabs pageItems[struct {
			Date  string `json:"date"`
			Items pageItems[struct {
				Tracks pageItems[historyItem] `json:"tracks"`
			}] `json:"items"`
		}] `json:"historyTabs"`
	}
	e := a.callBounded(ctx, "GET", "/music-history?fullModelsCount=0", nil, nil, &v, discoveryLimit)
	if e != nil {
		return nil, e
	}
	out := make([]HistoryRef, 0, 200)
	seen := map[ID]bool{}
	for _, tab := range v.Tabs {
		for _, group := range tab.Items {
			for _, item := range group.Tracks {
				id := item.Data.ID.TrackID
				if item.Type == "track" && id != "" && !seen[id] {
					out = append(out, HistoryRef{id, item.Data.ID.AlbumID, tab.Date})
					seen[id] = true
					if len(out) == 200 {
						return out, nil
					}
				}
			}
		}
	}
	return out, nil
}
func (a *API) HistoryTracks(ctx context.Context, refs []HistoryRef, page int) ([]Track, bool, error) {
	if a.Backend != nil {
		return a.Backend.HistoryTracks(ctx, refs, page)
	}
	start := page * PageSize
	if page < 0 || start >= len(refs) {
		return nil, false, nil
	}
	end := min(start+PageSize, len(refs))
	ids := make([]string, 0, end-start)
	for _, r := range refs[start:end] {
		id := string(r.ID)
		if r.AlbumID != "" {
			id += ":" + string(r.AlbumID)
		}
		ids = append(ids, id)
	}
	var tracks pageItems[Track]
	e := a.callBounded(ctx, "POST", "/tracks", url.Values{"track-ids": {strings.Join(ids, ",")}}, nil, &tracks, discoveryLimit)
	return tracks, end < len(refs), e
}
