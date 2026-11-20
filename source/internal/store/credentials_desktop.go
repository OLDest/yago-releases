//go:build !android

package store

import (
	"encoding/json"
	"errors"
	"github.com/zalando/go-keyring"
	"yago/internal/music"
)

const service = "YaGo.Desktop"

func Save(device string, t music.Token) error {
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	return keyring.Set(service, device, string(b))
}
func Load(device string) (music.Token, error) {
	var t music.Token
	s, e := keyring.Get(service, device)
	if e != nil {
		return t, e
	}
	e = json.Unmarshal([]byte(s), &t)
	if e == nil && t.Access == "" {
		e = errors.New("empty credential")
	}
	return t, e
}
func Delete(device string) error {
	e := keyring.Delete(service, device)
	if errors.Is(e, keyring.ErrNotFound) {
		return nil
	}
	return e
}
