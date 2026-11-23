//go:build !android

package store

import (
	"errors"
	"github.com/zalando/go-keyring"
)

func SaveUpdateToken(device, token string) error    { return keyring.Set("YaGo.Updates", device, token) }
func LoadUpdateToken(device string) (string, error) { return keyring.Get("YaGo.Updates", device) }
func DeleteUpdateToken(device string) error {
	e := keyring.Delete("YaGo.Updates", device)
	if errors.Is(e, keyring.ErrNotFound) {
		return nil
	}
	return e
}
