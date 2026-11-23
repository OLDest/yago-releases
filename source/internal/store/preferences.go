package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"yago/internal/music"
)

type Preferences struct {
	ZvukSeeds  map[string]string   `json:"zvukSeeds,omitempty"`
	Owner      music.ID            `json:"owner"`
	Artists    map[music.ID]string `json:"artists,omitempty"`
	Tracks     map[music.ID]bool   `json:"tracks,omitempty"`
	TrackNames map[music.ID]string `json:"trackNames,omitempty"`
	Seeds      map[string]string   `json:"seeds,omitempty"`
}

func preferenceFile(dir string, owner music.ID) string {
	sum := sha256.Sum256([]byte(owner))
	return filepath.Join(dir, "preferences-"+hex.EncodeToString(sum[:12])+".json")
}
func (p Preferences) Validate() error {
	if p.Owner == "" || len(p.Artists) > 2000 || len(p.Tracks) > 4000 || len(p.TrackNames) > 4000 || len(p.Seeds) > 8 || len(p.ZvukSeeds) > 8 {
		return errors.New("слишком много настроек")
	}
	for id, n := range p.Artists {
		if id == "" || len(id) > 128 || len(n) > 512 {
			return errors.New("неверный исполнитель")
		}
	}
	for id := range p.Tracks {
		if id == "" || len(id) > 128 {
			return errors.New("неверный трек")
		}
	}
	for id, name := range p.TrackNames {
		if id == "" || len(id) > 128 || len(name) > 512 {
			return errors.New("invalid blocked track name")
		}
	}
	for k, v := range p.ZvukSeeds {
		if len(k) > 64 || len(v) > 4096 {
			return errors.New("неверная настройка потока")
		}
	}
	for k, v := range p.Seeds {
		if len(k) > 64 || len(v) > 4096 {
			return errors.New("неверная настройка волны")
		}
	}
	return nil
}
func LoadPreferences(dir string, owner music.ID) (Preferences, error) {
	p := Preferences{Owner: owner}
	f, e := os.Open(preferenceFile(dir, owner))
	if errors.Is(e, os.ErrNotExist) {
		return p, nil
	}
	if e != nil {
		return p, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (512<<10)+1))
	if e != nil {
		return p, e
	}
	if len(b) > 512<<10 {
		return p, errors.New("настройки слишком велики")
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return Preferences{Owner: owner}, e
	}
	if p.Owner != owner {
		return Preferences{Owner: owner}, errors.New("чужие настройки")
	}
	if e = p.Validate(); e != nil {
		return Preferences{Owner: owner}, e
	}
	return p, nil
}
func SavePreferences(dir string, p Preferences) error {
	if e := p.Validate(); e != nil {
		return e
	}
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	if len(b) > 512<<10 {
		return errors.New("настройки слишком велики")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".preferences-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	if e = f.Chmod(0600); e != nil {
		return e
	}
	if _, e = f.Write(b); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, preferenceFile(dir, p.Owner))
}
func (p Preferences) Blocked(t music.Track) bool {
	if p.Tracks[t.ID] {
		return true
	}
	for _, a := range t.Artists {
		if _, ok := p.Artists[a.ID]; ok && a.ID != "" {
			return true
		}
	}
	return false
}
