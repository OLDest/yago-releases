package offline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
	"yago/internal/audio"
	"yago/internal/music"
)

type Resolver func(context.Context, music.Track, music.Quality) (music.StreamInfo, error)
type Job struct {
	Track    music.Track
	Playlist string
	Quality  music.Quality
	Resolve  Resolver
}
type Progress struct {
	Pending, Done, Failed int
	Downloaded, Total     int64
	Title, Error          string
	Active                bool
	Revision              uint64
}
type Manager struct {
	mu           sync.Mutex
	library      *Library
	ctx          context.Context
	cancel       context.CancelFunc
	activeCancel context.CancelFunc
	jobs         []Job
	current      *Job
	state        Progress
	revision     uint64
	wake         chan struct{}
	done         chan struct{}
	notify       func()
}

func NewManager(parent context.Context, library *Library, notify func()) *Manager {
	ctx, cancel := context.WithCancel(parent)
	m := &Manager{ctx: ctx, cancel: cancel, library: library, wake: make(chan struct{}, 1), done: make(chan struct{}), notify: notify}
	go m.run()
	return m
}
func (m *Manager) signal() {
	if m.notify != nil {
		m.notify()
	}
}
func (m *Manager) Snapshot() Progress {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.state
	p.Pending = len(m.jobs)
	p.Revision = m.revision
	return p
}
func (m *Manager) Enqueue(jobs []Job) (int, error) {
	m.mu.Lock()
	if m.ctx.Err() != nil {
		m.mu.Unlock()
		return 0, context.Canceled
	}
	if len(m.jobs)+len(jobs) > MaxTracks {
		m.mu.Unlock()
		return 0, errors.New("слишком длинная очередь загрузки")
	}
	if len(m.jobs) == 0 && m.current == nil {
		m.state = Progress{}
	}
	seen := map[string]bool{}
	key := func(j Job) string { return j.Playlist + "\x00" + string(j.Track.ID) }
	for _, j := range m.jobs {
		seen[key(j)] = true
	}
	if m.current != nil {
		seen[key(*m.current)] = true
	}
	added := 0
	for _, j := range jobs {
		if j.Playlist == "" {
			j.Playlist = DefaultID
		}
		if j.Track.ID == "" || seen[key(j)] {
			continue
		}
		seen[key(j)] = true
		m.jobs = append(m.jobs, j)
		added++
	}
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
	m.signal()
	return added, nil
}
func (m *Manager) Cancel() {
	m.mu.Lock()
	clear(m.jobs)
	m.jobs = nil
	if m.activeCancel != nil {
		m.activeCancel()
	}
	m.mu.Unlock()
	m.signal()
}
func (m *Manager) Close() { m.cancel(); m.Cancel(); <-m.done }
func (m *Manager) run() {
	defer close(m.done)
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.wake:
		}
		for {
			m.mu.Lock()
			if len(m.jobs) == 0 || m.ctx.Err() != nil {
				m.mu.Unlock()
				break
			}
			job := m.jobs[0]
			m.jobs[0] = Job{}
			m.jobs = m.jobs[1:]
			if len(m.jobs) == 0 {
				m.jobs = nil
			}
			ctx, cancel := context.WithCancel(m.ctx)
			m.activeCancel = cancel
			m.current = &job
			m.state.Active = true
			m.state.Title = job.Track.Title
			m.state.Downloaded = 0
			m.state.Total = 0
			m.mu.Unlock()
			m.signal()
			err := m.download(ctx, job)
			cancel()
			m.mu.Lock()
			m.activeCancel = nil
			m.current = nil
			m.state.Active = false
			m.revision++
			if err == nil {
				m.state.Done++
			} else if !errors.Is(err, context.Canceled) {
				m.state.Failed++
				m.state.Error = job.Track.Title + ": " + err.Error()
			}
			m.mu.Unlock()
			m.signal()
		}
	}
}
func safeAudioURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.User == nil && (u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "[::1]" || u.Hostname() == "::1")))
}
func (m *Manager) download(ctx context.Context, j Job) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := validateTrack(j.Track); e != nil {
		return e
	}
	if e := m.library.CanAdd(j.Playlist, j.Track.ID); e != nil {
		return e
	}
	if _, _, e := m.library.Resolve(j.Track.ID); e == nil {
		return m.library.Link(j.Playlist, j.Track.ID)
	}
	if j.Resolve == nil {
		return errors.New("для загрузки войдите в Яндекс")
	}
	info, e := j.Resolve(ctx, j.Track, j.Quality)
	if e != nil {
		return e
	}
	if !safeAudioURL(info.URL) {
		return errors.New("небезопасный адрес аудио")
	}
	request, e := http.NewRequestWithContext(ctx, "GET", info.URL, nil)
	if e != nil {
		return e
	}
	// A separate CDN client never receives API OAuth headers or cookies.
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 20 * time.Second, TLSHandshakeTimeout: 10 * time.Second}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !safeAudioURL(r.URL.String()) {
			return errors.New("небезопасное перенаправление аудио")
		}
		return nil
	}}
	defer client.CloseIdleConnections()
	response, e := client.Do(request)
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("нет соединения с аудиосервером")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("аудиосервер: HTTP %d", response.StatusCode)
	}
	if response.ContentLength > audio.MaxTrackBytes {
		return errors.New("трек превышает лимит 256 МиБ")
	}
	f, e := os.CreateTemp(filepath.Join(m.library.dir, "audio"), ".download-*.part")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	defer f.Close()
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	var total int64
	last := time.Time{}
	for {
		if e = ctx.Err(); e != nil {
			return e
		}
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > audio.MaxTrackBytes {
				return errors.New("трек превышает лимит 256 МиБ")
			}
			if _, e = io.MultiWriter(f, hash).Write(buffer[:n]); e != nil {
				return fmt.Errorf("не удалось записать файл: %w", e)
			}
			if time.Since(last) > 200*time.Millisecond {
				last = time.Now()
				m.mu.Lock()
				m.state.Downloaded = total
				m.state.Total = response.ContentLength
				m.mu.Unlock()
				m.signal()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("загрузка прервана")
		}
	}
	if total == 0 || (response.ContentLength >= 0 && total != response.ContentLength) {
		return errors.New("файл загружен не полностью")
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = audio.ValidateFile(temp); e != nil {
		return errors.New("сервер не вернул воспроизводимый MP3")
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	saved := Saved{Codec: info.Codec, Track: j.Track, Size: total, Bitrate: info.Bitrate, Quality: j.Quality, SHA256: hex.EncodeToString(hash.Sum(nil))}
	return m.library.commit(j.Playlist, saved, temp)
}
