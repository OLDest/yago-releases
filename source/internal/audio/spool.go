package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"
	"yago/internal/chromenet"
	"yago/internal/httpheaders"
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

// download starts streaming with the normal audio inactivity limit.
func (s *Spool) download(ctx context.Context, raw string) error {
	return s.downloadWithIdleTimeout(ctx, raw, 30*time.Second)
}

var errAudioIdle = errors.New("аудиосервер не передаёт данные более 30 секунд")
var audioErrorURL = regexp.MustCompile(`https?://[^\s"<>]+`)

// audioErrorDetail preserves network diagnostics without exposing signed media URLs.
func audioErrorDetail(err error) string {
	for {
		wrapped, ok := err.(*url.Error)
		if !ok {
			break
		}
		err = wrapped.Err
	}
	return audioErrorURL.ReplaceAllString(err.Error(), "[audio URL]")
}

// downloadWithIdleTimeout logs transport failures and retries once before any audio bytes are consumed.
func (s *Spool) downloadWithIdleTimeout(ctx context.Context, raw string, idle time.Duration) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return err
	}
	// Tokens are never attached to CDN requests.
	httpheaders.Media(req)
	client := &http.Client{Transport: chromenet.NewTransport(), CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("слишком много перенаправлений")
		}
		if r.URL.Scheme != "https" && r.URL.Hostname() != "127.0.0.1" {
			return errors.New("небезопасный адрес аудио")
		}
		return nil
	}}
	defer client.CloseIdleConnections()
	var resp *http.Response
	for attempt := 1; attempt <= 2; attempt++ {
		started := time.Now()
		attemptCtx, stopAttempt := context.WithCancel(ctx)
		defer stopAttempt()
		headerTimer := time.AfterFunc(12*time.Second, stopAttempt)
		resp, err = client.Do(req.Clone(attemptCtx))
		if !headerTimer.Stop() && err == nil {
			resp.Body.Close()
			resp = nil
			err = context.DeadlineExceeded
		}
		if ctx.Err() != nil {
			if resp != nil {
				resp.Body.Close()
			}
			return ctx.Err()
		}
		retry := err != nil
		if err != nil {
			log.Printf("audio connect host=%s attempt=%d elapsed=%s error_type=%T error=%s", req.URL.Hostname(), attempt, time.Since(started), err, audioErrorDetail(err))
		} else {
			log.Printf("audio response host=%s attempt=%d elapsed=%s status=%d protocol=%s bytes=%d", resp.Request.URL.Hostname(), attempt, time.Since(started), resp.StatusCode, resp.Proto, resp.ContentLength)
			retry = resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500
		}
		if !retry || attempt == 2 {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		client.CloseIdleConnections()
		log.Printf("audio retry host=%s next_attempt=2", req.URL.Hostname())
		delay := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			delay.Stop()
			return ctx.Err()
		case <-delay.C:
		}
	}
	if err != nil {
		return fmt.Errorf("не удалось подключиться к аудиосерверу после 2 попыток: %s", audioErrorDetail(err))
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
	watchdog := time.AfterFunc(idle, func() { cancel(errAudioIdle) })
	defer watchdog.Stop()
	buf := make([]byte, 64<<10)
	var received int64
	for {
		n, e := resp.Body.Read(buf)
		if n > 0 {
			if received == 0 {
				log.Printf("audio first_bytes host=%s bytes=%d", resp.Request.URL.Hostname(), n)
			}
			received += int64(n)
			watchdog.Reset(idle)
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
			log.Printf("audio complete host=%s bytes=%d", resp.Request.URL.Hostname(), received)
			return nil
		}
		if e != nil {
			log.Printf("audio read host=%s bytes=%d error_type=%T error=%s", resp.Request.URL.Hostname(), received, e, audioErrorDetail(e))
			if errors.Is(context.Cause(ctx), errAudioIdle) {
				return errAudioIdle
			}
			return fmt.Errorf("загрузка аудио прервана: %s", audioErrorDetail(e))
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
