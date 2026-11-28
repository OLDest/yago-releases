package store

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"yago/internal/music"
)

const maxPlaybackBytes = 4 << 20

type Playback struct {
	Offline         bool                `json:"offline,omitempty"`
	OfflinePlaylist string              `json:"offlinePlaylist,omitempty"`
	Version         int                 `json:"version"`
	Owner           music.ID            `json:"owner"`
	Demo            bool                `json:"demo,omitempty"`
	Queue           []music.Track       `json:"queue"`
	Order           []int               `json:"order"`
	Index           int                 `json:"index"`
	Position        float64             `json:"position"`
	Playing         bool                `json:"playing"`
	Stopped         bool                `json:"stopped"`
	Volume          float32             `json:"volume"`
	Muted           bool                `json:"muted"`
	Shuffled        bool                `json:"shuffled"`
	Wave            bool                `json:"wave"`
	Session         string              `json:"session,omitempty"`
	Batch           string              `json:"batch,omitempty"`
	WaveContexts    []music.WaveContext `json:"waveContexts,omitempty"`
}

func (p Playback) Validate() error {
	if len(p.WaveContexts) > 0 {
		if !p.Wave || len(p.WaveContexts) != len(p.Queue) {
			return errors.New("invalid wave queue metadata")
		}
		for _, c := range p.WaveContexts {
			if len(c.Session) > 4096 || len(c.Batch) > 4096 {
				return errors.New("invalid wave track context")
			}
		}
	}
	if p.Offline && (p.Demo || p.Owner != "__offline__" || p.Wave) {
		return errors.New("invalid offline checkpoint")
	}
	if len(p.OfflinePlaylist) > 128 {
		return errors.New("invalid offline playlist")
	}
	if p.Version != 1 || p.Owner == "" || len(p.Queue) == 0 || len(p.Queue) > 2000 || p.Index < 0 || p.Index >= len(p.Queue) || len(p.Order) != len(p.Queue) {
		return errors.New("invalid playback queue")
	}
	if math.IsNaN(p.Position) || math.IsInf(p.Position, 0) || p.Position < 0 || p.Position > 86400 || math.IsNaN(float64(p.Volume)) || p.Volume < 0 || p.Volume > 1 {
		return errors.New("invalid playback position/volume")
	}
	seen := make([]bool, len(p.Queue))
	for _, i := range p.Order {
		if i < 0 || i >= len(seen) || seen[i] {
			return errors.New("invalid playback order")
		}
		seen[i] = true
	}
	for _, t := range p.Queue {
		if t.ID == "" {
			return errors.New("empty track ID")
		}
	}
	if len(p.Session) > 4096 || len(p.Batch) > 4096 {
		return errors.New("invalid wave state")
	}
	return nil
}
func SavePlayback(dir string, p Playback) error {
	if e := p.Validate(); e != nil {
		return e
	}
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	if len(b) > maxPlaybackBytes {
		return errors.New("playback checkpoint too large")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".playback-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	if _, e = f.Write(b); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(dir, "playback.json"))
}
func LoadPlayback(dir string) (Playback, error) {
	var p Playback
	f, e := os.Open(filepath.Join(dir, "playback.json"))
	if e != nil {
		return p, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, maxPlaybackBytes+1))
	if e != nil {
		return p, e
	}
	if len(b) > maxPlaybackBytes {
		return p, errors.New("playback checkpoint too large")
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return p, e
	}
	return p, p.Validate()
}
func DeletePlayback(dir string) error {
	e := os.Remove(filepath.Join(dir, "playback.json"))
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	return e
}
