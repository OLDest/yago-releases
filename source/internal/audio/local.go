package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
)

// OpenLocal reads the saved MP3 in place. Closing playback never deletes it.
func OpenLocal(path string) (*Spool, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > MaxTrackBytes {
		f.Close()
		return nil, errors.New("неверный локальный аудиофайл")
	}
	finished := make(chan struct{})
	close(finished)
	s := &Spool{file: f, size: info.Size(), done: true, persistent: true, cancel: func() {}, finished: finished}
	s.cond = sync.NewCond(&s.mu)
	return s, nil
}
func OpenVerifiedLocal(ctx context.Context, path, expected string) (*Spool, error) {
	s, e := OpenLocal(path)
	if e != nil {
		return nil, e
	}
	hash := sha256.New()
	buf := make([]byte, 64<<10)
	var total int64
	for {
		if e = ctx.Err(); e != nil {
			break
		}
		n, readErr := s.file.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > MaxTrackBytes {
				e = errors.New("локальный файл слишком велик")
				break
			}
			_, _ = hash.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			e = readErr
			break
		}
	}
	if e == nil && (total != s.size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected)) {
		e = errors.New("локальный файл повреждён; скачайте трек заново")
	}
	if e == nil {
		_, e = s.file.Seek(0, io.SeekStart)
	}
	if e != nil {
		s.Close()
		return nil, e
	}
	return s, nil
}
func ValidateFile(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	// Hide seeking: validating one decoded frame does not build a full index.
	dec, e := newPCMDecoder(struct{ io.Reader }{f})
	if e == nil {
		var sample [4]byte
		_, e = io.ReadFull(dec, sample[:])
	}
	return e
}
