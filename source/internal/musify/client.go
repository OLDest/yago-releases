package musify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"yago/internal/chromenet"
)

const apiBase = "https://musify.club/api/mobile/v1/"
const userAgent = chromenet.UserAgent

var Diagnostics = "0"

type Client struct {
	token     atomic.Pointer[string]
	http      *http.Client
	installID string
}

func New(client *http.Client, installID string) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{http: client, installID: installID}
}

type Song struct {
	ID            int64
	Title         string
	Artist        string
	Album         string
	Cover         string
	DurationSec   int
	IsCopyrighted bool
	ArtistID      int64
	AlbumID       int64
	Artists       []Artist
}

type SearchResult struct {
	Query   string
	Songs   []Song
	Artists []Artist
	Albums  []Album
	Counts  map[string]int
	HasMore bool
}

type Stream struct {
	URL                 string
	ExpiresAt           string
	DailyPlaysRemaining *int
}

type HTTPError struct {
	Status     int
	RetryAfter string
}

func (e *HTTPError) Error() string {
	if e.Status == http.StatusForbidden {
		return "Musify отказал в доступе (HTTP 403). Причина отказа неизвестна."
	}
	return fmt.Sprintf("Musify API returned HTTP %d", e.Status)
}

func (c *Client) Search(ctx context.Context, query, kind string, page, limit int) (SearchResult, error) {
	var result SearchResult
	params := url.Values{"q": {strings.TrimSpace(query)}, "type": {kind}, "page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(limit)}}
	err := c.get(ctx, "search", params, &result)
	return result, err
}

func (c *Client) Home(ctx context.Context) (json.RawMessage, error) {
	return c.catalog(ctx, "home", nil)
}

func (c *Client) Suggest(ctx context.Context, query string) (json.RawMessage, error) {
	return c.catalog(ctx, "suggest", url.Values{"q": {strings.TrimSpace(query)}})
}

func (c *Client) Artist(ctx context.Context, id int64) (json.RawMessage, error) {
	return c.catalog(ctx, "artist/"+strconv.FormatInt(id, 10), nil)
}

func (c *Client) Album(ctx context.Context, id int64) (json.RawMessage, error) {
	return c.catalog(ctx, "album/"+strconv.FormatInt(id, 10), nil)
}

func (c *Client) StreamURL(ctx context.Context, id int64) (Stream, error) {
	var stream Stream
	if c.installID == "" {
		return stream, fmt.Errorf("Musify stream requires an installation ID")
	}
	err := c.get(ctx, "track/"+strconv.FormatInt(id, 10)+"/stream-url", nil, &stream)
	if err != nil {
		return Stream{}, err
	}
	media, err := url.Parse(stream.URL)
	if err != nil || media.Scheme != "https" || media.Hostname() == "" || media.User != nil {
		return Stream{}, fmt.Errorf("Musify returned an invalid stream URL")
	}
	return stream, nil
}

func (c *Client) catalog(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	var result json.RawMessage
	err := c.get(ctx, path, params, &result)
	return result, err
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	return c.request(ctx, http.MethodGet, path, params, nil, out)
}

func (c *Client) request(ctx context.Context, method, path string, params url.Values, input, out any) error {
	target := apiBase + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if c.installID != "" {
		req.Header.Set("X-Install-Id", c.installID)
	}
	operation := path
	if parts := strings.Split(path, "/"); len(parts) > 1 {
		parts[1] = "{id}"
		operation = strings.Join(parts, "/")
	}
	started := time.Now()
	token := c.token.Load()
	if token != nil {
		req.Header.Set("Authorization", "Bearer "+*token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		if Diagnostics == "1" {
			log.Printf("musify op=%s outcome=transport_error elapsed_ms=%d", operation, time.Since(started).Milliseconds())
		}
	} else if Diagnostics == "1" {
		log.Printf("musify op=%s signed_in=%t status=%d elapsed_ms=%d", operation, token != nil, res.StatusCode, time.Since(started).Milliseconds())
	}
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &HTTPError{Status: res.StatusCode, RetryAfter: res.Header.Get("Retry-After")}
	}
	if out == nil {
		return nil
	}
	err = json.NewDecoder(res.Body).Decode(out)
	if err != nil && Diagnostics == "1" {
		log.Printf("musify op=%s outcome=decode_error", operation)
	}
	return err
}

type ChartEntry struct {
	Rank int
	Song Song
}

type Charts struct {
	CompositeTitle string
	Entries        []ChartEntry
}

func (c *Client) Charts(ctx context.Context) (Charts, error) {
	var result Charts
	err := c.get(ctx, "charts", nil, &result)
	return result, err
}

type Artist struct {
	ID   int64
	Name string
}

type Album struct {
	ID    int64
	Title string
}

type Suggestion struct {
	Title string
}

func (c *Client) Suggestions(ctx context.Context, query string) ([]Suggestion, error) {
	var result struct{ Items []Suggestion }
	err := c.get(ctx, "suggest", url.Values{"q": {strings.TrimSpace(query)}}, &result)
	return result.Items, err
}

func (c *Client) AlbumTracks(ctx context.Context, id int64) ([]Song, error) {
	var result struct{ Tracks []Song }
	err := c.get(ctx, "album/"+strconv.FormatInt(id, 10), nil, &result)
	return result.Tracks, err
}

func (c *Client) ArtistTopSongs(ctx context.Context, id int64) ([]Song, error) {
	var result struct{ TopSongs []Song }
	err := c.get(ctx, "artist/"+strconv.FormatInt(id, 10), nil, &result)
	return result.TopSongs, err
}

func (c *Client) FileURL(ctx context.Context, id int64) (string, error) {
	var result struct{ URL string }
	if err := c.get(ctx, "track/"+strconv.FormatInt(id, 10)+"/file-url", nil, &result); err != nil {
		return "", err
	}
	media, err := url.Parse(result.URL)
	if err != nil || media.Scheme != "https" || media.Hostname() == "" || media.User != nil {
		return "", fmt.Errorf("Musify не вернул безопасную ссылку на файл")
	}
	return result.URL, nil
}

func (c *Client) Favorites(ctx context.Context) ([]Song, error) {
	if !c.SignedIn() {
		return nil, fmt.Errorf("Войдите в Musify")
	}
	const limit = 50
	var songs []Song
	for page := 0; page < 100; page++ {
		var result struct {
			Items      []Song
			TotalCount int
		}
		params := url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(limit)}}
		if err := c.get(ctx, "me/favorites/tracks", params, &result); err != nil {
			return nil, err
		}
		songs = append(songs, result.Items...)
		if len(result.Items) == 0 || len(songs) >= result.TotalCount {
			return songs, nil
		}
	}
	return nil, fmt.Errorf("Превышен лимит страниц избранного Musify")
}

func (c *Client) SetFavorite(ctx context.Context, id int64, add bool) error {
	if !c.SignedIn() {
		return fmt.Errorf("Войдите в Musify")
	}
	method := http.MethodDelete
	if add {
		method = http.MethodPut
	}
	return c.request(ctx, method, "me/favorites/tracks/"+strconv.FormatInt(id, 10), nil, nil, nil)
}

type Playlist struct {
	ID        int64
	Name      string
	SongCount int
}

func (c *Client) Playlists(ctx context.Context) ([]Playlist, error) {
	if !c.SignedIn() {
		return nil, fmt.Errorf("Войдите в Musify")
	}
	raw, err := c.catalog(ctx, "me/playlists", nil)
	if err != nil {
		return nil, err
	}
	var items []Playlist
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}
	var envelope struct {
		Items     *[]Playlist
		Playlists *[]Playlist
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Items != nil {
		return *envelope.Items, nil
	}
	if envelope.Playlists != nil {
		return *envelope.Playlists, nil
	}
	return nil, fmt.Errorf("Неизвестный формат списка плейлистов Musify")
}

func (c *Client) PlaylistTracks(ctx context.Context, id int64) ([]Song, error) {
	if !c.SignedIn() {
		return nil, fmt.Errorf("Войдите в Musify")
	}
	var item struct{ Tracks []Song }
	err := c.get(ctx, "me/playlists/"+strconv.FormatInt(id, 10), nil, &item)
	return item.Tracks, err
}

type ChartOption struct {
	ID   int64
	Name string
}

func (c *Client) ChartOptions(ctx context.Context) (genres, moods, themes []ChartOption, err error) {
	var result struct{ Genres, Tags, Themes []ChartOption }
	err = c.get(ctx, "browse", nil, &result)
	return result.Genres, result.Tags, result.Themes, err
}

const MaxChartMix = 6

func ValidChartSource(source string) bool {
	if !strings.Contains(source, ",") {
		return validChartFeed(source)
	}
	parts := strings.Split(source, ",")
	if len(parts) > MaxChartMix {
		return false
	}
	seen := map[string]bool{}
	for _, part := range parts {
		if !strings.Contains(part, ":") || !validChartFeed(part) || seen[part] {
			return false
		}
		seen[part] = true
	}
	return true
}

func validChartFeed(source string) bool {
	if source == "" || source == "week" || source == "month" {
		return true
	}
	kind, raw, ok := strings.Cut(source, ":")
	id, err := strconv.ParseInt(raw, 10, 64)
	return ok && err == nil && id > 0 && (kind == "genre" || kind == "mood" || kind == "theme")
}

func (c *Client) ChartPage(ctx context.Context, source, cursor string) (Charts, string, error) {
	if !ValidChartSource(source) {
		return Charts{}, "", fmt.Errorf("Неизвестная настройка чарта Musify")
	}
	if strings.Contains(source, ",") {
		return c.mixedChartPage(ctx, strings.Split(source, ","), cursor)
	}
	if id, ok := strings.CutPrefix(source, "theme:"); ok {
		return c.themeChartPage(ctx, id, cursor)
	}
	params := url.Values{}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if source == "" {
		var page struct {
			Charts
			NextCursor string
		}
		err := c.get(ctx, "charts/full", params, &page)
		return page.Charts, page.NextCursor, err
	}
	if id, ok := strings.CutPrefix(source, "mood:"); ok {
		return c.moodChartPage(ctx, id, cursor)
	}
	route := "charts/tracks"
	if id, ok := strings.CutPrefix(source, "genre:"); ok {
		route = "genre/" + id + "/top"
	} else {
		params.Set("period", source)
	}
	var page struct {
		Items      []Song
		NextCursor string
	}
	err := c.get(ctx, route, params, &page)
	return chartSongs(page.Items), page.NextCursor, err
}

func chartSongs(songs []Song) Charts {
	var out Charts
	for _, song := range songs {
		if song.ID > 0 {
			out.Entries = append(out.Entries, ChartEntry{Song: song})
		}
	}
	return out
}

func (c *Client) moodChartPage(ctx context.Context, id, cursor string) (Charts, string, error) {
	pageCursor, rawOffset, _ := strings.Cut(cursor, "|")
	offset, _ := strconv.Atoi(rawOffset)
	if offset < 0 || offset > 1000 {
		return Charts{}, "", fmt.Errorf("Неверная страница настроения Musify")
	}
	params := url.Values{}
	if pageCursor != "" {
		params.Set("cursor", pageCursor)
	}
	var page struct {
		Items      []Album
		NextCursor string
	}
	if err := c.get(ctx, "tag/"+id+"/albums", params, &page); err != nil {
		return Charts{}, "", err
	}
	var songs []Song
	end := min(offset+3, len(page.Items))
	for i := offset; i < end; i++ {
		if page.Items[i].ID <= 0 {
			continue
		}
		tracks, err := c.AlbumTracks(ctx, page.Items[i].ID)
		if err != nil {
			return Charts{}, "", err
		}
		songs = append(songs, tracks...)
	}
	next := ""
	if end < len(page.Items) {
		next = pageCursor + "|" + strconv.Itoa(end)
	} else if page.NextCursor != "" && page.NextCursor != pageCursor {
		next = page.NextCursor + "|0"
	}
	return chartSongs(songs), next, nil
}

type HistoryItem struct {
	Song     Song
	PlayedAt string
}

func (c *Client) HistoryPage(ctx context.Context, cursor string) ([]HistoryItem, string, error) {
	var result struct {
		Items  []HistoryItem
		Cursor string
	}
	if !c.SignedIn() {
		return nil, "", fmt.Errorf("Войдите в Musify")
	}
	params := url.Values{"limit": {"50"}}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	err := c.get(ctx, "me/history", params, &result)
	return result.Items, result.Cursor, err
}

func (c *Client) EditPlaylist(ctx context.Context, operation string, id, song int64, name string) error {
	if !c.SignedIn() {
		return fmt.Errorf("Войдите в Musify")
	}
	const base = "me/playlists"
	item := base + "/" + strconv.FormatInt(id, 10)
	switch operation {
	case "create":
		return c.request(ctx, http.MethodPost, base, nil, map[string]string{"name": name, "description": ""}, nil)
	case "rename":
		return c.request(ctx, http.MethodPatch, item, nil, map[string]string{"name": name}, nil)
	case "delete":
		return c.request(ctx, http.MethodDelete, item, nil, nil, nil)
	case "add":
		return c.request(ctx, http.MethodPost, item+"/tracks", nil, map[string]int64{"songId": song}, nil)
	case "remove":
		return c.request(ctx, http.MethodDelete, item+"/tracks/"+strconv.FormatInt(song, 10), nil, nil, nil)
	default:
		return fmt.Errorf("Неизвестная операция с плейлистом Musify")
	}
}

type chartFeedCursor struct {
	Next string
	Done bool
}

func (c *Client) mixedChartPage(ctx context.Context, feeds []string, cursor string) (Charts, string, error) {
	state := make([]chartFeedCursor, len(feeds))
	if cursor != "" {
		if len(cursor) > 2500 || json.Unmarshal([]byte(cursor), &state) != nil || len(state) != len(feeds) {
			return Charts{}, "", fmt.Errorf("Неверная страница подборок Musify")
		}
	}
	pages := make([][]ChartEntry, len(feeds))
	var errs []error
	maxRows := 0
	for i, source := range feeds {
		if state[i].Done {
			continue
		}
		page, next, err := c.ChartPage(ctx, source, state[i].Next)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		pages[i] = page.Entries
		maxRows = max(maxRows, len(page.Entries))
		state[i] = chartFeedCursor{Next: next, Done: next == "" || next == state[i].Next}
	}
	if err := ctx.Err(); err != nil {
		return Charts{}, "", err
	}
	var result Charts
	seen := map[int64]bool{}
	for row := 0; row < maxRows; row++ {
		for _, page := range pages {
			if row >= len(page) {
				continue
			}
			entry := page[row]
			if entry.Song.ID <= 0 || seen[entry.Song.ID] {
				continue
			}
			seen[entry.Song.ID] = true
			entry.Rank = len(result.Entries) + 1
			result.Entries = append(result.Entries, entry)
		}
	}
	if len(result.Entries) == 0 && len(errs) > 0 {
		return Charts{}, "", errors.Join(errs...)
	}
	for _, feed := range state {
		if !feed.Done {
			raw, _ := json.Marshal(state)
			return result, string(raw), nil
		}
	}
	return result, "", nil
}

func (c *Client) themeChartPage(ctx context.Context, id, cursor string) (Charts, string, error) {
	offset := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 0 {
			return Charts{}, "", fmt.Errorf("Неверная страница плейлистов Musify")
		}
		offset = n
	}
	var theme struct{ Playlists []Playlist }
	if err := c.get(ctx, "theme/"+id, nil, &theme); err != nil {
		return Charts{}, "", err
	}
	if offset >= len(theme.Playlists) {
		return Charts{}, "", nil
	}
	var item struct{ Tracks []Song }
	if err := c.get(ctx, "me/playlists/"+strconv.FormatInt(theme.Playlists[offset].ID, 10), nil, &item); err != nil {
		return Charts{}, "", err
	}
	next := ""
	if offset+1 < len(theme.Playlists) {
		next = strconv.Itoa(offset + 1)
	}
	return chartSongs(item.Tracks), next, nil
}
