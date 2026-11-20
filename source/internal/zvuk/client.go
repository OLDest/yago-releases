// Package zvuk implements a small, bounded HTTP client. No web renderer is used for playback.
package zvuk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"yago/internal/music"
)

const Origin = "https://zvuk.com"
const prefix = "zvuk-"
const responseLimit = 2 << 20

type Client struct {
	AccountHint *music.Account
	Token       string
	Base        string
	HTTP        *http.Client
	mu          sync.Mutex
	searchKey   string
	// Keep only the cursor needed for the next/previous page, never search responses.
	cursors      map[int][4]string
	waveFeedback *waveFeedback
	waveSerial   uint64
}

var _ music.Backend = (*Client)(nil)

func New(token string) *Client {
	tr := &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 20 * time.Second, TLSHandshakeTimeout: 10 * time.Second}
	return &Client{Token: token, Base: Origin, HTTP: &http.Client{Transport: tr, Timeout: 25 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		// API redirects are not authentication; never forward a token to another origin.
		return http.ErrUseLastResponse
	}}}
}
func (c *Client) Close() { c.HTTP.CloseIdleConnections() }

// CachedIdentity describes a saved profile; it does not assert current server access.
func (c *Client) CachedIdentity() music.Account {
	if c.AccountHint != nil && music.IsZvuk(c.AccountHint.UID) {
		return *c.AccountHint
	}
	sum := sha256.Sum256([]byte(c.Token))
	return music.Account{UID: music.ID(fmt.Sprintf("zvuk-account-%x", sum[:12])), DisplayName: "Звук", Login: "zvuk"}
}
func ID(raw music.ID) (string, error) {
	s := string(raw)
	if !strings.HasPrefix(s, prefix) {
		return "", errors.New("Трек принадлежит другому сервису")
	}
	s = strings.TrimPrefix(s, prefix)
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	if len(s) == 0 || len(s) > 100 {
		return "", errors.New("Некорректный ID Звука")
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_') {
			return "", errors.New("Некорректный ID Звука")
		}
	}
	return s, nil
}
func ns(id music.ID) music.ID {
	if id == "" {
		return ""
	}
	return music.ID(prefix + string(id))
}
func (c *Client) request(ctx context.Context, path string, body any, out any) error {
	var reader io.Reader
	method := "GET"
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		reader = bytes.NewReader(b)
		method = "POST"
	}
	req, e := http.NewRequestWithContext(ctx, method, c.Base+path, reader)
	if e != nil {
		return errors.New("Некорректный запрос Звука")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", Origin)
	req.Header.Set("Referer", Origin+"/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36")
	req.Header.Set("X-Auth-Token", c.Token)
	resp, e := c.HTTP.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("Нет соединения со Звуком")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 418 {
		return errors.New("Звук заблокировал запрос защитой сайта. Повторите позже")
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("Звук отклонил доступ. Проверьте вход и подписку")
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Звук: HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if e != nil {
		return errors.New("Ответ Звука не получен")
	}
	if len(b) > responseLimit {
		return errors.New("Слишком большой ответ Звука")
	}
	if !json.Valid(b) {
		return errors.New("Звук вернул страницу сайта вместо данных")
	}
	if e = json.Unmarshal(b, out); e != nil {
		return errors.New("Неизвестный формат ответа Звука")
	}
	return nil
}
func (c *Client) gql(ctx context.Context, query string, vars any, out any) error {
	var env struct {
		Data   json.RawMessage
		Errors []json.RawMessage
	}
	if e := c.request(ctx, "/api/v1/graphql", map[string]any{"query": query, "variables": vars}, &env); e != nil {
		return e
	}
	if len(env.Errors) > 0 {
		return errors.New("Звук отклонил операцию API. Проверьте вход и доступность функции")
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return errors.New("Пустой ответ Звука")
	}
	if out == nil {
		return nil
	}
	if e := json.Unmarshal(env.Data, out); e != nil {
		return errors.New("Неизвестный формат данных Звука")
	}
	return nil
}

type Profile struct {
	ID        music.ID `json:"id"`
	Name      string   `json:"name"`
	Token     string   `json:"token"`
	Anonymous *bool    `json:"is_anonymous"`
}

func ParseProfile(b []byte) (Profile, error) {
	var env struct{ Result json.RawMessage }
	if len(b) > 64<<10 || json.Unmarshal(b, &env) != nil {
		return Profile{}, errors.New("Некорректный профиль Звука")
	}
	var p Profile
	if json.Unmarshal(env.Result, &p) != nil {
		return p, errors.New("Профиль Звука не получен")
	}
	var nested struct {
		Profile *Profile `json:"profile"`
	}
	_ = json.Unmarshal(env.Result, &nested)
	if nested.Profile != nil {
		p = *nested.Profile
	}
	if p.ID == "" || p.Anonymous == nil || *p.Anonymous {
		return Profile{}, errors.New("Вход в Звук ещё не выполнен")
	}
	return p, nil
}
func (c *Client) Account(ctx context.Context) (music.Account, error) {
	var last error
	for _, path := range []string{"/api/v2/tiny/profile", "/api/tiny/profile"} {
		var raw json.RawMessage
		if e := c.request(ctx, path, nil, &raw); e != nil {
			last = e
			continue
		}
		p, e := ParseProfile(raw)
		if e != nil {
			last = e
			continue
		}
		name := p.Name
		if name == "" {
			name = "Звук"
		}
		return music.Account{UID: ns(p.ID), DisplayName: name, Login: "zvuk"}, nil
	}
	// zvuk-music/Music Assistant also verify the authenticated collection when
	// ServicePipe blocks Tiny profile. Preserve a previously authenticated identity.
	var collection struct {
		Collection *struct{ Playlists []struct{ ID music.ID } }
	}
	if e := c.gql(ctx, `query{collection{playlists{id}}}`, nil, &collection); e == nil && collection.Collection != nil {
		return c.CachedIdentity(), nil
	}
	return music.Account{}, last
}

const trackFields = `id title duration artists { id title } release { id title }`

type named struct {
	ID    music.ID
	Title string
}
type track struct {
	ID       music.ID
	Title    string
	Duration int64
	Artists  []named
	Release  named
}

func (t track) model() music.Track {
	v := music.Track{ID: ns(t.ID), Title: t.Title, DurationMS: t.Duration * 1000}
	for _, a := range t.Artists {
		v.Artists = append(v.Artists, music.Named{ID: ns(a.ID), Name: a.Title})
	}
	if t.Release.ID != "" {
		v.Albums = []music.Album{{ID: ns(t.Release.ID), Title: t.Release.Title}}
	}
	return v
}
func models(ts []track) []music.Track {
	v := make([]music.Track, 0, len(ts))
	for _, t := range ts {
		if t.ID != "" {
			v = append(v, t.model())
		}
	}
	return v
}
func rawIDs(ids []string) ([]string, error) {
	v := make([]string, 0, len(ids))
	for _, id := range ids {
		s, e := ID(music.ID(id))
		if e != nil {
			return nil, e
		}
		v = append(v, s)
	}
	return v, nil
}
func (c *Client) Tracks(ctx context.Context, ids []string) ([]music.Track, error) {
	if len(ids) > 2000 {
		return nil, errors.New("Слишком много треков")
	}
	var result []music.Track
	for i := 0; i < len(ids); i += music.PageSize {
		raw, e := rawIDs(ids[i:min(i+music.PageSize, len(ids))])
		if e != nil {
			return nil, e
		}
		var v struct {
			Tracks []track `json:"getTracks"`
		}
		if e = c.gql(ctx, `query($ids:[ID!]!){getTracks(ids:$ids){`+trackFields+`}}`, map[string]any{"ids": raw}, &v); e != nil {
			return nil, e
		}
		result = append(result, models(v.Tracks)...)
	}
	return result, nil
}

type group[T any] struct {
	Items []T
	Page  struct {
		Next   *int
		Cursor string
	}
}

func (c *Client) Search(ctx context.Context, q string) ([]music.Track, error) {
	v, e := c.SearchPage(ctx, q, "track", 0)
	return v.Tracks, e
}
func (c *Client) SearchPage(ctx context.Context, q, kind string, page int) (music.SearchPage, error) {
	if page < 0 || page > 1000 || len(q) > 1000 {
		return music.SearchPage{}, errors.New("Некорректный запрос поиска")
	}
	if kind != "all" && kind != "track" && kind != "artist" && kind != "album" && kind != "playlist" {
		return music.SearchPage{}, errors.New("Неизвестный тип поиска")
	}
	key := q + "\x00" + kind
	c.mu.Lock()
	if c.searchKey != key || page == 0 {
		c.searchKey = key
		c.cursors = map[int][4]string{0: {}}
	}
	cur, ok := c.cursors[page]
	c.mu.Unlock()
	if !ok {
		// Rebuild evicted cursors on demand. Only a few cursor strings stay resident.
		r, e := c.SearchPage(ctx, q, kind, 0)
		for p := 1; e == nil && p <= page; p++ {
			if !r.More {
				return music.SearchPage{}, nil
			}
			r, e = c.SearchPage(ctx, q, kind, p)
		}
		return r, e
	}
	vars := map[string]any{"q": q, "limit": music.PageSize}
	names := []string{"tc", "ac", "rc", "pc"}
	for i, n := range names {
		if cur[i] != "" {
			vars[n] = cur[i]
		}
	}
	var v struct {
		Search struct {
			Tracks    group[track]
			Artists   group[named]
			Releases  group[named]
			Playlists group[named]
		}
	}
	query := `query($q:String,$limit:Int,$tc:Cursor,$ac:Cursor,$rc:Cursor,$pc:Cursor){search(query:$q){`
	active := [4]bool{}
	for i, k := range []string{"track", "artist", "album", "playlist"} {
		active[i] = (kind == "all" || kind == k) && cur[i] != "!end"
	}
	if active[0] {
		query += `tracks(limit:$limit,cursor:$tc){page{next cursor} items{` + trackFields + `}}`
	}
	if active[1] {
		query += `artists(limit:$limit,cursor:$ac){page{next cursor} items{id title}}`
	}
	if active[2] {
		query += `releases(limit:$limit,cursor:$rc){page{next cursor} items{id title}}`
	}
	if active[3] {
		query += `playlists(limit:$limit,cursor:$pc){page{next cursor} items{id title}}`
	}
	query += `}}`
	// GraphQL disallows unused variable declarations.
	for i := range names {
		if !active[i] {
			query = strings.Replace(query, ",$"+names[i]+":Cursor", "", 1)
			delete(vars, names[i])
		}
	}
	if e := c.gql(ctx, query, vars, &v); e != nil {
		return music.SearchPage{}, e
	}
	r := music.SearchPage{Tracks: models(v.Search.Tracks.Items)}
	for _, a := range v.Search.Artists.Items {
		r.Artists = append(r.Artists, music.Named{ID: ns(a.ID), Name: a.Title})
	}
	for _, a := range v.Search.Releases.Items {
		r.Albums = append(r.Albums, music.Album{ID: ns(a.ID), Title: a.Title})
	}
	for _, a := range v.Search.Playlists.Items {
		r.Playlists = append(r.Playlists, music.Playlist{Kind: ns(a.ID), Title: a.Title})
	}
	next := [4]string{v.Search.Tracks.Page.Cursor, v.Search.Artists.Page.Cursor, v.Search.Releases.Page.Cursor, v.Search.Playlists.Page.Cursor}
	for i, s := range next {
		if len(s) > 4096 {
			return music.SearchPage{}, errors.New("Некорректный курсор поиска Звука")
		}
		if s == "" {
			next[i] = "!end"
		} else {
			r.More = true
		}
	}
	c.mu.Lock()
	if c.searchKey == key {
		if len(c.cursors) >= 8 {
			for p := range c.cursors {
				if p != page && p != page-1 {
					delete(c.cursors, p)
				}
			}
		}
		if r.More {
			c.cursors[page+1] = next
		}
	}
	c.mu.Unlock()
	return r, nil
}
func (c *Client) Likes(ctx context.Context, _ music.ID) ([]music.Track, error) {
	var v struct{ Collection struct{ Tracks []named } }
	e := c.gql(ctx, `query{collection{tracks(orderBy:dateAdded,orderDirection:desc){id}}}`, nil, &v)
	if e != nil {
		return nil, e
	}
	// Match the existing library ceiling; keep IDs only until each batch is decoded.
	if len(v.Collection.Tracks) > 2000 {
		v.Collection.Tracks = v.Collection.Tracks[:2000]
	}
	ids := make([]string, 0, len(v.Collection.Tracks))
	for _, t := range v.Collection.Tracks {
		ids = append(ids, string(ns(t.ID)))
	}
	return c.Tracks(ctx, ids)
}
func (c *Client) Playlists(ctx context.Context, _ music.ID) ([]music.Playlist, error) {
	var v struct{ Collection struct{ Playlists []named } }
	if e := c.gql(ctx, `query{collection{playlists{id}}}`, nil, &v); e != nil {
		return nil, e
	}
	var result []music.Playlist
	for i := 0; i < min(len(v.Collection.Playlists), 200); i += music.PageSize {
		ids := []string{}
		for _, p := range v.Collection.Playlists[i:min(i+music.PageSize, min(len(v.Collection.Playlists), 200))] {
			ids = append(ids, string(p.ID))
		}
		var data struct {
			Playlists []struct {
				ID     music.ID
				Title  string
				UserID music.ID
			} `json:"getPlaylists"`
		}
		if e := c.gql(ctx, `query($ids:[ID!]!){getPlaylists(ids:$ids){id title userId}}`, map[string]any{"ids": ids}, &data); e != nil {
			return nil, e
		}
		for _, p := range data.Playlists {
			x := music.Playlist{Kind: ns(p.ID), Title: p.Title}
			x.Owner.UID = ns(p.UserID)
			result = append(result, x)
		}
	}
	return result, nil
}
func (c *Client) Playlist(ctx context.Context, _ music.ID, id music.ID) ([]music.Track, error) {
	raw, e := ID(id)
	if e != nil {
		return nil, e
	}
	var all []music.Track
	for offset := 0; offset < 2000; offset += music.PageSize {
		var v struct {
			Tracks []track `json:"playlistTracks"`
		}
		e = c.gql(ctx, `query($id:ID!,$limit:Int,$offset:Int){playlistTracks(id:$id,limit:$limit,offset:$offset){`+trackFields+`}}`, map[string]any{"id": raw, "limit": music.PageSize, "offset": offset}, &v)
		if e != nil {
			return nil, e
		}
		all = append(all, models(v.Tracks)...)
		if len(v.Tracks) < music.PageSize {
			break
		}
	}
	return all, nil
}
func (c *Client) collection(ctx context.Context, id music.ID, kind string, hidden, remove bool) error {
	raw, e := ID(id)
	if e != nil {
		return e
	}
	if kind != "track" && kind != "artist" {
		return errors.New("Неизвестный тип объекта")
	}
	root, action := "collection", "addItem"
	if hidden {
		root = "hidden_collection"
	}
	if remove {
		action = "removeItem"
	}
	return c.gql(ctx, `mutation($id:ID!,$type:CollectionItemType!){`+root+`{`+action+`(id:$id,type:$type)}}`, map[string]any{"id": raw, "type": kind}, nil)
}
func (c *Client) Like(ctx context.Context, _ music.ID, t music.Track) error {
	return c.collection(ctx, t.ID, "track", false, false)
}
func (c *Client) Unlike(ctx context.Context, _ music.ID, t music.Track) error {
	return c.collection(ctx, t.ID, "track", false, true)
}
func (c *Client) Dislike(ctx context.Context, _ music.ID, kind string, id music.ID, remove bool) error {
	return c.collection(ctx, id, kind, true, remove)
}
func (c *Client) Stream(ctx context.Context, t music.Track, q music.Quality) (music.StreamInfo, error) {
	id, e := ID(t.ID)
	if e != nil {
		return music.StreamInfo{}, e
	}
	// The Music Assistant provider uses these MP3 fields directly. Only FLAC/DRM
	// variants require another decoder/container; do not reject the whole GraphQL API.
	var data struct {
		Contents []struct {
			Stream struct{ High, Mid, Flac string }
		} `json:"mediaContents"`
	}
	gqlErr := c.gql(ctx, `query($ids:[ID!]!){mediaContents(ids:$ids){... on Track{stream{high mid flac}}}}`, map[string]any{"ids": []string{id}}, &data)
	if gqlErr == nil && len(data.Contents) > 0 {
		s := data.Contents[0].Stream
		if q == music.Lossless {
			if validStreamURL(s.Flac) {
				return music.StreamInfo{URL: s.Flac, Codec: "flac", Available: []music.Quality{music.Normal, music.High, music.Lossless}}, nil
			}
			return music.StreamInfo{}, music.ErrLosslessUnavailable
		}
		available := []music.Quality{}
		if validStreamURL(s.Mid) {
			available = append(available, music.Normal)
		}
		if validStreamURL(s.High) {
			available = append(available, music.High)
		}
		raw, bitrate := s.Mid, 128
		if q == music.High && validStreamURL(s.High) {
			raw, bitrate = s.High, 320
		}
		if validStreamURL(raw) {
			return music.StreamInfo{URL: raw, Bitrate: bitrate, Available: available}, nil
		}
	}
	if ctx.Err() != nil {
		return music.StreamInfo{}, ctx.Err()
	}
	if q == music.Lossless {
		if gqlErr != nil {
			return music.StreamInfo{}, gqlErr
		}
		return music.StreamInfo{}, music.ErrLosslessUnavailable
	}
	mode, bitrate := "high", 320
	if q != music.High {
		mode, bitrate = "mid", 128
	}
	// Legacy Tiny remains a fallback, rather than a mandatory prerequisite.
	var v struct {
		Result struct {
			Link string `json:"stream"`
		}
	}
	e = c.request(ctx, "/api/tiny/track/stream?"+url.Values{"id": {id}, "quality": {mode}}.Encode(), nil, &v)
	if e != nil {
		if gqlErr != nil {
			return music.StreamInfo{}, errors.Join(gqlErr, e)
		}
		return music.StreamInfo{}, e
	}
	if !validStreamURL(v.Result.Link) {
		return music.StreamInfo{}, errors.New("Звук не выдал прямой MP3. Проверьте подписку и качество")
	}
	return music.StreamInfo{URL: v.Result.Link, Bitrate: bitrate, Available: []music.Quality{music.Normal, music.High}}, nil
}
func validStreamURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}
func (c *Client) AlbumPage(ctx context.Context, id music.ID, page int) ([]music.Track, bool, error) {
	if page < 0 || page > 50 {
		return nil, false, errors.New("Некорректная страница")
	}
	raw, e := ID(id)
	if e != nil {
		return nil, false, e
	}
	var v struct {
		Releases []struct{ Tracks []track } `json:"getReleases"`
	}
	if e = c.gql(ctx, `query($ids:[ID!]!){getReleases(ids:$ids){tracks{`+trackFields+`}}}`, map[string]any{"ids": []string{raw}}, &v); e != nil {
		return nil, false, e
	}
	if len(v.Releases) == 0 {
		return nil, false, nil
	}
	ts := v.Releases[0].Tracks
	start := min(page*music.PageSize, len(ts))
	end := min(start+music.PageSize, len(ts))
	return models(ts[start:end]), end < len(ts), nil
}
func (c *Client) ArtistTracks(ctx context.Context, id music.ID, page int) ([]music.Track, bool, error) {
	if page < 0 || page > 1000 {
		return nil, false, errors.New("Некорректная страница")
	}
	raw, e := ID(id)
	if e != nil {
		return nil, false, e
	}
	var v struct {
		Artists []struct {
			Tracks []track `json:"popularTracks"`
		} `json:"getArtists"`
	}
	e = c.gql(ctx, `query($ids:[ID!]!,$offset:Int,$limit:Int){getArtists(ids:$ids){popularTracks(offset:$offset,limit:$limit){`+trackFields+`}}}`, map[string]any{"ids": []string{raw}, "offset": page * music.PageSize, "limit": music.PageSize}, &v)
	if e != nil || len(v.Artists) == 0 {
		return nil, false, e
	}
	ts := models(v.Artists[0].Tracks)
	return ts, len(ts) == music.PageSize, nil
}
func (c *Client) History(ctx context.Context) ([]music.HistoryRef, error) {
	var v struct {
		History []struct {
			Date  string                `json:"lastListeningDttm"`
			Track struct{ ID music.ID } `json:"mediaContent"`
		} `json:"listeningHistory"`
	}
	e := c.gql(ctx, `query{listeningHistory(limit:200,offset:0){lastListeningDttm mediaContent{... on Track{id}}}}`, nil, &v)
	if e != nil {
		return nil, e
	}
	var refs []music.HistoryRef
	for _, h := range v.History {
		if h.Track.ID != "" {
			refs = append(refs, music.HistoryRef{ID: ns(h.Track.ID), Date: h.Date})
		}
	}
	return refs, nil
}
func (c *Client) HistoryTracks(ctx context.Context, refs []music.HistoryRef, page int) ([]music.Track, bool, error) {
	if page < 0 || page > 1000 {
		return nil, false, errors.New("Некорректная страница")
	}
	start := min(page*music.PageSize, len(refs))
	end := min(start+music.PageSize, len(refs))
	ids := []string{}
	for _, r := range refs[start:end] {
		ids = append(ids, string(r.ID))
	}
	ts, e := c.Tracks(ctx, ids)
	return ts, end < len(refs), e
}
func (c *Client) Suggest(ctx context.Context, q string) ([]string, error) {
	var v struct {
		Search struct{ Content []struct{ Title string } } `json:"quickSearch"`
	}
	e := c.gql(ctx, `query($q:String,$limit:Int){quickSearch(query:$q,limit:$limit){content{... on Track{title} ... on Artist{title} ... on Release{title}}}}`, map[string]any{"q": q, "limit": 5}, &v)
	var out []string
	for _, t := range v.Search.Content {
		if t.Title != "" && len(out) < 5 {
			out = append(out, t.Title)
		}
	}
	return out, e
}
