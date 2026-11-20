package store

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

func Device(dir string) (string, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	path := filepath.Join(dir, "device-id")
	if b, e := os.ReadFile(path); e == nil && len(b) == 32 {
		return string(b), nil
	}
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	id := hex.EncodeToString(b[:])
	if e := os.WriteFile(path, []byte(id), 0600); e != nil {
		return "", e
	}
	return id, nil
}
