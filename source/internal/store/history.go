package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
	"yago/internal/music"
)

const HistoryLimit = 200

type Heard struct {
	Track music.Track `json:"track"`
	At    time.Time   `json:"at"`
}

func historyFile(dir string, owner music.ID) string {
	sum := sha256.Sum256([]byte(owner))
	return filepath.Join(dir, "history-"+hex.EncodeToString(sum[:12])+".json")
}
func LoadHistory(dir string, owner music.ID) ([]Heard, error) {
	if owner == "" {
		return nil, nil
	}
	f, e := os.Open(historyFile(dir, owner))
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (512<<10)+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 512<<10 {
		return nil, errors.New("История слишком велика")
	}
	var items []Heard
	if e = json.Unmarshal(b, &items); e != nil {
		return nil, e
	}
	if len(items) > HistoryLimit {
		return nil, errors.New("История слишком велика")
	}
	return items, nil
}
func RecordHistory(dir string, owner music.ID, track music.Track, at time.Time) error {
	if owner == "" || track.ID == "" {
		return nil
	}
	old, e := LoadHistory(dir, owner)
	if e != nil {
		return e
	}
	// History stores small metadata only; never artwork, URLs, or audio.
	track.CoverURI = ""
	track.Available = nil
	next := make([]Heard, 0, min(HistoryLimit, len(old)+1))
	next = append(next, Heard{Track: track, At: at.UTC()})
	for _, h := range old {
		if h.Track.ID != track.ID && len(next) < HistoryLimit {
			next = append(next, h)
		}
	}
	b, e := json.Marshal(next)
	if e != nil {
		return e
	}
	if len(b) > 512<<10 {
		return errors.New("История слишком велика")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".history-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, historyFile(dir, owner))
}
