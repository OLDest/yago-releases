package music

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"yago/internal/musify"
)

type Musify struct {
	chartSource atomic.Pointer[string]
	Client      *musify.Client
	historyMu   sync.RWMutex
	history     map[ID]Track
}

var _ Backend = (*Musify)(nil)

var errMusifyUnsupported = errors.New("Эта функция пока недоступна в Musify")

func IsMusify(id ID) bool { return strings.HasPrefix(string(id), "musify-") }

func musifyID(id int64) ID {
	if id <= 0 {
		return ""
	}
	return ID("musify-" + strconv.FormatInt(id, 10))
}

func parseMusifyID(id ID) (int64, error) {
	value, ok := strings.CutPrefix(string(id), "musify-")
	n, err := strconv.ParseInt(value, 10, 64)
	if !ok || err != nil || n <= 0 {
		return 0, errors.New("Неверный идентификатор Musify")
	}
	return n, nil
}

func musifyTrack(song musify.Song) Track {
	t := Track{
		ID: musifyID(song.ID), Title: song.Title,
		DurationMS: int64(song.DurationSec) * 1000, CoverURI: song.Cover,
	}
	for _, artist := range song.Artists {
		t.Artists = append(t.Artists, Named{ID: musifyID(artist.ID), Name: artist.Name})
	}
	if len(t.Artists) == 0 && song.Artist != "" {
		t.Artists = []Named{{ID: musifyID(song.ArtistID), Name: song.Artist}}
	}
	if id := musifyID(song.AlbumID); id != "" {
		t.Albums = []Album{{ID: id, Title: song.Album}}
	}
	return t
}

func (m *Musify) Account(context.Context) (Account, error) {
	return Account{UID: "musify-guest", DisplayName: "Musify · гость"}, nil
}

func (m *Musify) Search(ctx context.Context, query string) ([]Track, error) {
	result, err := m.SearchPage(ctx, query, "track", 0)
	return result.Tracks, err
}

func (m *Musify) SearchPage(ctx context.Context, query, kind string, page int) (SearchPage, error) {
	kinds := map[string]string{"track": "songs", "album": "albums", "artist": "artists", "all": "all"}
	filter, ok := kinds[kind]
	if !ok {
		return SearchPage{}, errMusifyUnsupported
	}
	if page < 0 {
		return SearchPage{}, errors.New("Неверная страница поиска Musify")
	}
	result, err := m.Client.Search(ctx, query, filter, page+1, PageSize)
	if err != nil {
		return SearchPage{}, err
	}
	out := SearchPage{More: result.HasMore}
	for _, song := range result.Songs {
		if song.ID > 0 {
			out.Tracks = append(out.Tracks, musifyTrack(song))
		}
	}
	for _, album := range result.Albums {
		out.Albums = append(out.Albums, Album{ID: musifyID(album.ID), Title: album.Title})
	}
	for _, artist := range result.Artists {
		out.Artists = append(out.Artists, Named{ID: musifyID(artist.ID), Name: artist.Name})
	}
	return out, nil
}

func (m *Musify) Suggest(ctx context.Context, query string) ([]string, error) {
	items, err := m.Client.Suggestions(ctx, query)
	var titles []string
	for _, item := range items {
		if item.Title != "" {
			titles = append(titles, item.Title)
		}
	}
	return titles, err
}

func (m *Musify) Charts(ctx context.Context) ([]Track, error) {
	var charts musify.Charts
	var err error
	if source := m.ChartSource(); source != "" {
		charts, _, err = m.Client.ChartPage(ctx, source, "")
	} else {
		charts, err = m.Client.Charts(ctx)
	}
	var tracks []Track
	for _, entry := range charts.Entries {
		if entry.Song.ID > 0 {
			tracks = append(tracks, musifyTrack(entry.Song))
		}
	}
	return tracks, err
}

func (m *Musify) Stream(ctx context.Context, track Track, _ Quality) (StreamInfo, error) {
	id, err := parseMusifyID(track.ID)
	if err != nil {
		return StreamInfo{}, err
	}
	stream, err := m.Client.StreamURL(ctx, id)
	return StreamInfo{URL: stream.URL}, err
}

func (m *Musify) AlbumPage(ctx context.Context, id ID, page int) ([]Track, bool, error) {
	idValue, err := parseMusifyID(id)
	if err != nil || page > 0 {
		return nil, false, err
	}
	result, err := m.Client.AlbumTracks(ctx, idValue)
	return musifyTracks(result), false, err
}

func (m *Musify) ArtistTracks(ctx context.Context, id ID, page int) ([]Track, bool, error) {
	idValue, err := parseMusifyID(id)
	if err != nil || page > 0 {
		return nil, false, err
	}
	result, err := m.Client.ArtistTopSongs(ctx, idValue)
	return musifyTracks(result), false, err
}

func musifyTracks(songs []musify.Song) []Track {
	tracks := make([]Track, 0, len(songs))
	for _, song := range songs {
		if song.ID > 0 {
			tracks = append(tracks, musifyTrack(song))
		}
	}
	return tracks
}

func (m *Musify) Tracks(_ context.Context, ids []string) ([]Track, error) {
	m.historyMu.RLock()
	defer m.historyMu.RUnlock()
	out := make([]Track, 0, len(ids))
	for _, id := range ids {
		key, _, _ := strings.Cut(id, ":")
		if track, ok := m.history[ID(key)]; ok {
			out = append(out, track)
		}
	}
	return out, nil
}
func (m *Musify) ClearHistoryCache() {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()
	m.history = nil
}
func (m *Musify) Likes(ctx context.Context, _ ID) ([]Track, error) {
	songs, err := m.Client.Favorites(ctx)
	return musifyTracks(songs), err
}
func (m *Musify) Playlists(ctx context.Context, uid ID) ([]Playlist, error) {
	items, err := m.Client.Playlists(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Playlist, 0, len(items))
	for _, item := range items {
		id := musifyID(item.ID)
		if id == "" {
			continue
		}
		p := Playlist{Kind: id, Title: item.Name, TrackCount: item.SongCount}
		p.Owner.UID = uid
		out = append(out, p)
	}
	return out, nil
}
func (m *Musify) Playlist(ctx context.Context, _ ID, id ID) ([]Track, error) {
	n, err := parseMusifyID(id)
	if err != nil {
		return nil, err
	}
	songs, err := m.Client.PlaylistTracks(ctx, n)
	return musifyTracks(songs), err
}
func (m *Musify) Like(ctx context.Context, _ ID, track Track) error {
	id, err := parseMusifyID(track.ID)
	if err != nil {
		return err
	}
	return m.Client.SetFavorite(ctx, id, true)
}
func (m *Musify) Unlike(ctx context.Context, _ ID, track Track) error {
	id, err := parseMusifyID(track.ID)
	if err != nil {
		return err
	}
	return m.Client.SetFavorite(ctx, id, false)
}
func (*Musify) Dislike(context.Context, ID, string, ID, bool) error { return errMusifyUnsupported }
func (m *Musify) History(ctx context.Context) ([]HistoryRef, error) {
	const limit = 200
	refs := make([]HistoryRef, 0, limit)
	tracks := make(map[ID]Track)
	cursor := ""
	for page := 0; page < 4 && len(refs) < limit; page++ {
		items, next, err := m.Client.HistoryPage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			track := musifyTrack(item.Song)
			if track.ID == "" {
				continue
			}
			if _, ok := tracks[track.ID]; ok {
				continue
			}
			tracks[track.ID] = track
			refs = append(refs, HistoryRef{ID: track.ID, Date: item.PlayedAt})
			if len(refs) == limit {
				break
			}
		}
		if next == "" || next == cursor || len(items) == 0 {
			break
		}
		cursor = next
	}
	m.historyMu.Lock()
	m.history = tracks
	m.historyMu.Unlock()
	return refs, nil
}
func (m *Musify) HistoryTracks(ctx context.Context, refs []HistoryRef, page int) ([]Track, bool, error) {
	if page < 0 || page > len(refs)/PageSize || page*PageSize >= len(refs) {
		return nil, false, nil
	}
	end := min((page+1)*PageSize, len(refs))
	ids := make([]string, 0, PageSize)
	for _, ref := range refs[page*PageSize : end] {
		ids = append(ids, string(ref.ID))
	}
	tracks, err := m.Tracks(ctx, ids)
	return tracks, end < len(refs), err
}
func (*Musify) WaveWithSeeds(context.Context, string, []string, []string) (Wave, error) {
	return Wave{}, errMusifyUnsupported
}
func (*Musify) Feedback(context.Context, string, string, string, Track, float64) error {
	return errMusifyUnsupported
}
func (*Musify) WaveOptions(context.Context) ([]WaveOption, error) { return nil, errMusifyUnsupported }

func (m *Musify) Download(ctx context.Context, track Track, _ Quality) (StreamInfo, error) {
	id, err := parseMusifyID(track.ID)
	if err != nil {
		return StreamInfo{}, err
	}
	raw, err := m.Client.FileURL(ctx, id)
	return StreamInfo{URL: raw}, err
}

func (m *Musify) ChartSource() string {
	if v := m.chartSource.Load(); v != nil {
		return *v
	}
	return ""
}

func (m *Musify) SetChartSource(source string) {
	if source != "" && musify.ValidChartSource(source) {
		m.chartSource.Store(&source)
	} else {
		m.chartSource.Store(nil)
	}
}
