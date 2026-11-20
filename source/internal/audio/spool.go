package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// A growing disk file keeps compressed audio out of the Go heap. Readers wait
// for the next bytes; playback does not wait for the complete download.
type Spool struct {
	persistent bool
	mu         sync.Mutex
	cond       *sync.Cond
	file       *os.File
	size       int64
	done       bool
	err        error
	cancel     context.CancelFunc
	finished   chan struct{}
}

const MaxTrackBytes int64 = 256 << 20

func OpenStream(parent context.Context, raw, dir string) (*Spool, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, "audio-*.part")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Spool{file: f, cancel: cancel, finished: make(chan struct{})}
	s.cond = sync.NewCond(&s.mu)
	go func() {
		defer close(s.finished)
		err := s.download(ctx, raw)
		s.mu.Lock()
		s.done = true
		s.err = err
		s.cond.Broadcast()
		s.mu.Unlock()
	}()
	return s, nil
}
func (s *Spool) download(ctx context.Context, raw string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return err
	}
	// Tokens are never attached to CDN requests.
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 20 * time.Second, TLSHandshakeTimeout: 10 * time.Second}, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("слишком много перенаправлений")
		}
		if r.URL.Scheme != "https" && r.URL.Hostname() != "127.0.0.1" {
			return errors.New("небезопасный адрес аудио")
		}
		return nil
	}}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("не удалось подключиться к аудиосерверу")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("аудиосервер: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxTrackBytes {
		return errors.New("трек превышает лимит 256 МиБ")
	}
	// The deadline is per download; context cancellation interrupts a stalled body.
	timer := time.AfterFunc(15*time.Minute, s.cancel)
	defer timer.Stop()
	buf := make([]byte, 64<<10)
	for {
		n, e := resp.Body.Read(buf)
		if n > 0 {
			s.mu.Lock()
			size := s.size
			s.mu.Unlock()
			if size+int64(n) > MaxTrackBytes {
				return errors.New("трек превышает лимит 256 МиБ")
			}
			if _, err = s.file.WriteAt(buf[:n], size); err != nil {
				return err
			}
			s.mu.Lock()
			s.size += int64(n)
			s.cond.Broadcast()
			s.mu.Unlock()
		}
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return errors.New("загрузка аудио прервана")
		}
	}
}

type streamReader struct {
	s      *Spool
	ctx    context.Context
	offset int64
}

func (s *Spool) Reader(ctx context.Context) io.Reader { return &streamReader{s: s, ctx: ctx} }

// Completed downloads can be indexed and seeked without decoding the prefix.
func (s *Spool) SeekableReader(ctx context.Context) io.ReadSeeker {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.done || s.err != nil {
		return nil
	}
	return &streamSeeker{streamReader: &streamReader{s: s, ctx: ctx}}
}

type streamSeeker struct{ *streamReader }

func (r *streamSeeker) Seek(offset int64, whence int) (int64, error) {
	size, _, _ := r.s.Status()
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += r.offset
	case io.SeekEnd:
		offset += size
	default:
		return 0, errors.New("invalid seek origin")
	}
	if offset < 0 {
		return 0, errors.New("negative seek")
	}
	r.offset = offset
	return offset, nil
}
func (r *streamReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s := r.s
	s.mu.Lock()
	if r.offset >= s.size && !s.done && r.ctx.Err() == nil {
		// Only a blocking read needs a cancellation wakeup. Cached reads create none.
		stop := context.AfterFunc(r.ctx, func() { s.mu.Lock(); s.cond.Broadcast(); s.mu.Unlock() })
		defer stop()
		for r.offset >= s.size && !s.done && r.ctx.Err() == nil {
			s.cond.Wait()
		}
	}
	if err := r.ctx.Err(); err != nil {
		s.mu.Unlock()
		return 0, err
	}
	available := s.size - r.offset
	if available <= 0 {
		err := s.err
		if err == nil {
			err = io.EOF
		}
		s.mu.Unlock()
		return 0, err
	}
	if int64(len(p)) > available {
		p = p[:available]
	}
	s.mu.Unlock()
	n, err := s.file.ReadAt(p, r.offset)
	r.offset += int64(n)
	return n, err
}
func (s *Spool) Close() {
	s.cancel()
	<-s.finished
	s.mu.Lock()
	s.done = true
	s.cond.Broadcast()
	s.mu.Unlock()
	name := s.file.Name()
	s.file.Close()
	if !s.persistent {
		os.Remove(name)
	}
}
func (s *Spool) Status() (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.size, s.done, s.err
}
