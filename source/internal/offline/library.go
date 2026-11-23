package offline

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
	"yago/internal/music"
)

const DefaultID = "offline"
const maxManifest = 16 << 20
const MaxTracks = 20000
const MaxPlaylistTracks = 2000
const maxPlaylists = 256

type Saved struct {
	Filename string        `json:"filename,omitempty"`
	Track    music.Track   `json:"track"`
	Size     int64         `json:"size"`
	Codec    string        `json:"codec,omitempty"`
	Bitrate  int           `json:"bitrate"`
	Quality  music.Quality `json:"quality"`
	SHA256   string        `json:"sha256"`
}
type Playlist struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Tracks []music.ID `json:"tracks"`
}
type Summary struct {
	ID, Name string
	Count    int
	Bytes    int64
}
type manifest struct {
	GarbageFiles []string           `json:"garbageFiles,omitempty"`
	Garbage      []music.ID         `json:"garbage,omitempty"`
	Version      int                `json:"version"`
	Playlists    []Playlist         `json:"playlists"`
	Tracks       map[music.ID]Saved `json:"tracks"`
}
type Library struct {
	mu   sync.Mutex
	dir  string
	data manifest
}

func Open(dir string) (*Library, error) {
	if e := os.MkdirAll(filepath.Join(dir, "audio"), 0700); e != nil {
		return nil, e
	}
	l := &Library{dir: dir, data: manifest{Version: 1, Playlists: []Playlist{{ID: DefaultID, Name: "Оффлайн"}}, Tracks: make(map[music.ID]Saved)}}
	f, e := os.Open(filepath.Join(dir, "library.json"))
	if e == nil {
		b, readErr := io.ReadAll(io.LimitReader(f, maxManifest+1))
		f.Close()
		if readErr != nil {
			return nil, readErr
		}
		if len(b) > maxManifest {
			return nil, errors.New("оффлайн-библиотека слишком велика")
		}
		if e = json.Unmarshal(b, &l.data); e != nil {
			return nil, fmt.Errorf("не удалось прочитать оффлайн-библиотеку: %w", e)
		}
		if e = l.validate(); e != nil {
			return nil, e
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	// Only our own interrupted temporary downloads are disposable. Keep MP3s.
	entries, _ := os.ReadDir(filepath.Join(dir, "audio"))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".download-") && strings.HasSuffix(entry.Name(), ".part") {
			_ = os.Remove(filepath.Join(dir, "audio", entry.Name()))
		}
	}
	if l.cleanGarbageLocked() {
		_ = l.writeLocked()
	}
	metadataParts, _ := os.ReadDir(dir)
	for _, entry := range metadataParts {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".library-") {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	return l, nil
}
func (l *Library) cleanGarbageLocked() bool {
	files := l.data.GarbageFiles
	l.data.GarbageFiles = nil
	for _, name := range files {
		if !validAudioName(name) {
			continue
		}
		if e := os.Remove(filepath.Join(l.dir, "audio", name)); e != nil && !os.IsNotExist(e) {
			l.data.GarbageFiles = append(l.data.GarbageFiles, name)
		}
	}
	old := l.data.Garbage
	var remaining []music.ID
	for _, id := range old {
		if _, ok := l.data.Tracks[id]; ok {
			continue
		}
		if e := os.Remove(l.path(id)); e != nil && !os.IsNotExist(e) {
			remaining = append(remaining, id)
		}
	}
	l.data.Garbage = remaining
	return len(old) != len(remaining) || len(files) != len(l.data.GarbageFiles)
}
func (l *Library) validate() error {
	if l.data.Version != 1 || l.data.Tracks == nil || len(l.data.Tracks) > MaxTracks || len(l.data.Garbage) > MaxTracks || len(l.data.Playlists) == 0 || len(l.data.Playlists) > maxPlaylists {
		return errors.New("неверный формат оффлайн-библиотеки")
	}
	seen := map[string]bool{}
	for _, p := range l.data.Playlists {
		if p.ID == "" || len(p.ID) > 128 || seen[p.ID] || len(p.Tracks) > MaxPlaylistTracks || !validName(p.Name) {
			return errors.New("неверный оффлайн-плейлист")
		}
		seen[p.ID] = true
		ids := map[music.ID]bool{}
		for _, id := range p.Tracks {
			if _, ok := l.data.Tracks[id]; !ok || ids[id] {
				return errors.New("неверные треки плейлиста")
			}
			ids[id] = true
		}
	}
	if !seen[DefaultID] {
		return errors.New("нет плейлиста Оффлайн")
	}
	for id, s := range l.data.Tracks {
		if s.Filename != "" && !validAudioName(s.Filename) {
			return errors.New("Неверное имя аудиофайла")
		}
		if validateTrack(s.Track) != nil || s.Track.ID != id || s.Size <= 0 || s.Size > 256<<20 || len(s.SHA256) != 64 {
			return errors.New("неверный сохранённый трек")
		}
		if hash, e := hex.DecodeString(s.SHA256); e != nil || len(hash) != 32 {
			return errors.New("неверный хеш сохранённого трека")
		}
	}
	for _, id := range l.data.Garbage {
		if len(id) == 0 || len(id) > 256 {
			return errors.New("неверная запись удаления")
		}
	}
	if len(l.data.GarbageFiles) > MaxTracks {
		return errors.New("Слишком много файлов удаления")
	}
	for _, name := range l.data.GarbageFiles {
		if !validAudioName(name) {
			return errors.New("Неверное имя удаляемого файла")
		}
	}
	return nil
}
func validateTrack(t music.Track) error {
	if len(t.ID) == 0 || len(t.ID) > 256 || len(t.Title) > 4096 || len(t.Artists) > 64 || len(t.Albums) > 64 || t.DurationMS < 0 {
		return errors.New("неверные данные трека")
	}
	for _, a := range t.Artists {
		if len(a.Name) > 1024 {
			return errors.New("слишком длинное имя исполнителя")
		}
	}
	for _, a := range t.Albums {
		if len(a.ID) > 256 {
			return errors.New("неверные данные альбома")
		}
	}
	return nil
}
func compactTrack(t music.Track) music.Track {
	t.CoverURI = ""
	t.Available = nil
	t.Artists = append([]music.Named(nil), t.Artists...)
	for i := range t.Artists {
		t.Artists[i].ID = ""
	}
	t.Albums = append([]music.Album(nil), t.Albums...)
	for i := range t.Albums {
		t.Albums[i].Title = ""
	}
	return t
}
func (l *Library) CanAdd(playlist string, id music.ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.playlistLocked(playlist)
	if p == nil {
		return errors.New("плейлист не найден")
	}
	if _, exists := l.data.Tracks[id]; !exists && len(l.data.Tracks) >= MaxTracks {
		return errors.New("слишком много сохранённых треков")
	}
	if !contains(p.Tracks, id) && len(p.Tracks) >= MaxPlaylistTracks {
		return errors.New("в плейлисте уже 2000 треков")
	}
	return nil
}
func validName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 80
}
func (l *Library) path(id music.ID) string {
	if s, ok := l.data.Tracks[id]; ok && validAudioName(s.Filename) {
		return filepath.Join(l.dir, "audio", s.Filename)
	}
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(l.dir, "audio", hex.EncodeToString(sum[:])+".mp3")
}
func (l *Library) writeLocked() error {
	f, e := os.CreateTemp(l.dir, ".library-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	buffer := bufio.NewWriterSize(f, 64<<10)
	if e = l.encodeLocked(&manifestWriter{w: buffer, left: maxManifest}); e != nil {
		return e
	}
	if e = buffer.Flush(); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	info, e := os.Stat(name)
	if e != nil {
		return e
	}
	if info.Size() > maxManifest {
		return errors.New("оффлайн-библиотека слишком велика")
	}
	return os.Rename(name, filepath.Join(l.dir, "library.json"))
}

type manifestWriter struct {
	w    io.Writer
	left int
}

func (w *manifestWriter) Write(b []byte) (int, error) {
	if len(b) > w.left {
		return 0, errors.New("оффлайн-библиотека слишком велика")
	}
	n, e := w.w.Write(b)
	w.left -= n
	return n, e
}

// Encode one playlist/track at a time; never marshal the entire catalogue
// into another large byte slice for each completed song.
func (l *Library) encodeLocked(w io.Writer) error {
	encoder := json.NewEncoder(w)
	put := func(s string) error { _, e := io.WriteString(w, s); return e }
	if e := put("{\"version\":1,\"playlists\":["); e != nil {
		return e
	}
	for i, p := range l.data.Playlists {
		if i > 0 {
			if e := put(","); e != nil {
				return e
			}
		}
		if e := encoder.Encode(p); e != nil {
			return e
		}
	}
	if e := put("],\"tracks\":{"); e != nil {
		return e
	}
	first := true
	for id, saved := range l.data.Tracks {
		if !first {
			if e := put(","); e != nil {
				return e
			}
		}
		first = false
		if e := encoder.Encode(string(id)); e != nil {
			return e
		}
		if e := put(":"); e != nil {
			return e
		}
		if e := encoder.Encode(saved); e != nil {
			return e
		}
	}
	if e := put("},\"garbage\":["); e != nil {
		return e
	}
	for i, id := range l.data.Garbage {
		if i > 0 {
			if e := put(","); e != nil {
				return e
			}
		}
		if e := encoder.Encode(id); e != nil {
			return e
		}
	}
	if e := put("],\"garbageFiles\":"); e != nil {
		return e
	}
	if e := encoder.Encode(l.data.GarbageFiles); e != nil {
		return e
	}
	return put("}\n")
}
func (l *Library) playlistLocked(id string) *Playlist {
	if id == "" {
		id = DefaultID
	}
	for i := range l.data.Playlists {
		if l.data.Playlists[i].ID == id {
			return &l.data.Playlists[i]
		}
	}
	return nil
}
func (l *Library) Summaries() []Summary {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Summary, 0, len(l.data.Playlists))
	for _, p := range l.data.Playlists {
		s := Summary{ID: p.ID, Name: p.Name, Count: len(p.Tracks)}
		for _, id := range p.Tracks {
			s.Bytes += l.data.Tracks[id].Size
		}
		out = append(out, s)
	}
	return out
}
func (l *Library) Tracks(playlist string) []music.Track {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.playlistLocked(playlist)
	if p == nil {
		return nil
	}
	out := make([]music.Track, 0, len(p.Tracks))
	for _, id := range p.Tracks {
		out = append(out, l.data.Tracks[id].Track)
	}
	return out
}
func (l *Library) Create(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !validName(name) {
		return "", errors.New("название: от 1 до 80 символов")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, p := range l.data.Playlists {
		if strings.EqualFold(p.Name, name) {
			return "", errors.New("такой плейлист уже есть")
		}
	}
	if len(l.data.Playlists) >= maxPlaylists {
		return "", errors.New("слишком много плейлистов")
	}
	var idBytes [16]byte
	if _, e := rand.Read(idBytes[:]); e != nil {
		return "", e
	}
	id := hex.EncodeToString(idBytes[:])
	l.data.Playlists = append(l.data.Playlists, Playlist{ID: id, Name: name})
	if e := l.writeLocked(); e != nil {
		l.data.Playlists = l.data.Playlists[:len(l.data.Playlists)-1]
		return "", e
	}
	return id, nil
}
func (l *Library) Resolve(id music.ID) (string, Saved, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.resolveLocked(id)
}
func (l *Library) resolveLocked(id music.ID) (string, Saved, error) {
	s, ok := l.data.Tracks[id]
	if !ok {
		return "", Saved{}, os.ErrNotExist
	}
	path := l.path(id)
	info, e := os.Stat(path)
	if e != nil {
		return "", s, e
	}
	if !info.Mode().IsRegular() || info.Size() != s.Size {
		return "", s, errors.New("локальный файл повреждён")
	}
	return path, s, nil
}
func contains(ids []music.ID, id music.ID) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// Link reuses one verified file across playlists. Empty destination means Оффлайн.
func (l *Library) Link(playlist string, id music.ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, _, e := l.resolveLocked(id); e != nil {
		return e
	}
	return l.linkLocked(playlist, id)
}
func (l *Library) linkLocked(playlist string, id music.ID) error {
	p := l.playlistLocked(playlist)
	if p == nil {
		return errors.New("плейлист не найден")
	}
	if contains(p.Tracks, id) {
		return nil
	}
	if len(p.Tracks) >= MaxPlaylistTracks {
		return errors.New("в плейлисте уже 2000 треков")
	}
	p.Tracks = append(p.Tracks, id)
	if e := l.writeLocked(); e != nil {
		p.Tracks = p.Tracks[:len(p.Tracks)-1]
		return e
	}
	return nil
}
func (l *Library) commit(playlist string, saved Saved, temp string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.playlistLocked(playlist) == nil {
		return errors.New("плейлист не найден")
	}
	if e := validateTrack(saved.Track); e != nil {
		return e
	}
	pDest := l.playlistLocked(playlist)
	if !contains(pDest.Tracks, saved.Track.ID) && len(pDest.Tracks) >= MaxPlaylistTracks {
		return errors.New("в плейлисте уже 2000 треков")
	}
	saved.Track = compactTrack(saved.Track)
	if _, _, e := l.resolveLocked(saved.Track.ID); e == nil {
		return l.linkLocked(playlist, saved.Track.ID)
	}
	if _, exists := l.data.Tracks[saved.Track.ID]; !exists && len(l.data.Tracks) >= MaxTracks {
		return errors.New("слишком много сохранённых треков")
	}
	l.cleanGarbageLocked()
	name, e := l.availableName(saved)
	if e != nil {
		return e
	}
	saved.Filename = name
	path := filepath.Join(l.dir, "audio", name)
	// Publish only the new, verified temporary download. Existing saved tracks
	// returned above keep their original filename; there is no rename migration.
	if e := os.Rename(temp, path); e != nil {
		return e
	}
	old, existed := l.data.Tracks[saved.Track.ID]
	l.data.Tracks[saved.Track.ID] = saved
	p := l.playlistLocked(playlist)
	oldIDs := p.Tracks
	if !contains(p.Tracks, saved.Track.ID) {
		p.Tracks = append(p.Tracks, saved.Track.ID)
	}
	if e := l.writeLocked(); e != nil {
		p.Tracks = oldIDs
		if existed {
			l.data.Tracks[saved.Track.ID] = old
		} else {
			delete(l.data.Tracks, saved.Track.ID)
		}
		_ = os.Remove(path)
		return e
	}
	return nil
}

// Removing one membership preserves the audio while another playlist uses it.
func (l *Library) Remove(playlist string, id music.ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.playlistLocked(playlist)
	if p == nil {
		return errors.New("плейлист не найден")
	}
	at := -1
	for i, v := range p.Tracks {
		if v == id {
			at = i
			break
		}
	}
	if at < 0 {
		return nil
	}
	old := p.Tracks
	ids := append([]music.ID(nil), old[:at]...)
	ids = append(ids, old[at+1:]...)
	p.Tracks = ids
	used := false
	for _, v := range l.data.Playlists {
		used = used || contains(v.Tracks, id)
	}
	saved := l.data.Tracks[id]
	path := l.path(id)
	oldFiles := l.data.GarbageFiles
	oldGarbage := l.data.Garbage
	if !used {
		delete(l.data.Tracks, id)
		if saved.Filename != "" {
			l.data.GarbageFiles = append(append([]string(nil), oldFiles...), saved.Filename)
		} else if !contains(l.data.Garbage, id) {
			l.data.Garbage = append(l.data.Garbage, id)
		}
	}
	if e := l.writeLocked(); e != nil {
		p.Tracks = old
		l.data.Garbage = oldGarbage
		l.data.GarbageFiles = oldFiles
		if !used {
			l.data.Tracks[id] = saved
		}
		return e
	}
	if !used {
		_ = os.Remove(path)
	}
	return nil
}
