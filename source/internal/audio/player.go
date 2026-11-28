package audio

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/oto/v3"
)

const Rate = 44100

type Snapshot struct {
	ID                      uint64
	Loading, Playing, Ended bool
	Paused                  bool
	Position                float64
	Error                   string
	DeviceError             bool
	Downloaded              int64
}
type command struct {
	filePath        string
	fileSHA256      string
	id              uint64
	kind            string
	paused          bool
	url             func(context.Context) (string, error)
	seconds, volume float64
}
type ready struct {
	output      *oto.Context
	gen         uint64
	p           *oto.Player
	counter     *counter
	err         error
	deviceError bool
	offset      float64
}
type Player struct {
	workers  sync.WaitGroup
	sequence atomic.Uint64
	commands chan command
	results  chan ready
	mu       sync.Mutex
	state    Snapshot
	dir      string
	notify   func()
	changes  chan struct{}
	done     chan struct{}
}

// The actor owns this cancellation function and releases it on every track
// switch, stop, and shutdown (through cleanup).
func trackContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func New(dir string, notify func()) *Player {
	p := &Player{commands: make(chan command, 32), results: make(chan ready), dir: dir, notify: notify, changes: make(chan struct{}, 1), done: make(chan struct{})}
	go p.run()
	return p
}
func (p *Player) send(c command) {
	select {
	case p.commands <- c:
	case <-p.done:
	}
}
func (p *Player) Play(url func(context.Context) (string, error)) uint64 {
	return p.PlayAt(url, 0, false)
}
func (p *Player) PlayAt(url func(context.Context) (string, error), seconds float64, paused bool) uint64 {
	return p.play(command{url: url, seconds: seconds, paused: paused})
}
func (p *Player) PlayFileAt(path string, seconds float64, paused bool) uint64 {
	return p.play(command{filePath: path, seconds: seconds, paused: paused})
}
func (p *Player) PlayVerifiedFileAt(path, hash string, seconds float64, paused bool) uint64 {
	return p.play(command{filePath: path, fileSHA256: hash, seconds: seconds, paused: paused})
}
func (p *Player) play(c command) uint64 {
	seconds := c.seconds
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		seconds = 0
	}
	id := p.sequence.Add(1)
	c.kind = "play"
	c.id = id
	c.seconds = seconds
	p.send(c)
	return id
}
func (p *Player) Toggle()                  { p.send(command{kind: "toggle"}) }
func (p *Player) Pause()                   { p.send(command{kind: "pause"}) }
func (p *Player) Resume()                  { p.send(command{kind: "resume"}) }
func (p *Player) Seek(s float64)           { p.send(command{kind: "seek", seconds: s}) }
func (p *Player) Volume(v float64)         { p.send(command{kind: "volume", volume: v}) }
func (p *Player) Stop()                    { p.send(command{kind: "stop"}) }
func (p *Player) Close()                   { p.send(command{kind: "close"}); <-p.done; p.workers.Wait() }
func (p *Player) Snapshot() Snapshot       { p.mu.Lock(); defer p.mu.Unlock(); return p.state }
func (p *Player) Changes() <-chan struct{} { return p.changes }
func (p *Player) set(s Snapshot) {
	p.mu.Lock()
	old := p.state
	if old == s {
		p.mu.Unlock()
		return
	}
	p.state = s
	p.mu.Unlock()
	a, b := old, s
	a.Position, b.Position, a.Downloaded, b.Downloaded = 0, 0, 0, 0
	if a != b {
		select {
		case p.changes <- struct{}{}:
		default:
		}
	}
	if p.notify != nil {
		p.notify()
	}
}

var contextOnce sync.Once
var output *oto.Context
var outputErr error

func device() (*oto.Context, error) {
	contextOnce.Do(func() {
		var ready chan struct{}
		output, ready, outputErr = oto.NewContext(&oto.NewContextOptions{SampleRate: Rate, ChannelCount: 2, Format: oto.FormatSignedInt16LE, BufferSize: 80 * time.Millisecond})
		if outputErr == nil {
			select {
			case <-ready:
				outputErr = output.Err()
			case <-time.After(8 * time.Second):
				outputErr = errors.New("аудиоустройство не ответило")
			}
		}
	})
	return output, outputErr
}

type counter struct {
	r      io.Reader
	n      atomic.Int64
	eof    atomic.Bool
	failed atomic.Bool
}

func (c *counter) Read(b []byte) (int, error) {
	n, e := c.r.Read(b)
	c.n.Add(int64(n))
	if e == io.EOF {
		c.eof.Store(true)
	} else if e != nil {
		c.failed.Store(true)
	}
	return n, e
}
func (p *Player) run() {
	defer close(p.done)
	tick := time.NewTicker(250 * time.Millisecond)
	tick.Stop()
	defer tick.Stop()
	var tickC <-chan time.Time
	var gen uint64
	var rootCancel context.CancelFunc
	var decodeCancel context.CancelFunc
	var spool *Spool
	var active *oto.Player
	var count *counter
	var offset float64
	volume := 0.65
	paused := false
	state := Snapshot{}
	var power outputPower
	setPower := func(running bool) {
		if runtime.GOOS == "android" {
			if err := power.set(running); err != nil {
				state.Error = "Ошибка аудиоустройства: " + err.Error()
				state.DeviceError, state.Playing = true, false
			}
		}
	}
	syncTick := func() {
		if active != nil && !paused {
			if tickC == nil {
				tick.Reset(250 * time.Millisecond)
				tickC = tick.C
			}
		} else if tickC != nil {
			tick.Stop()
			tickC = nil
		}
	}
	closeOutput := func() {
		if decodeCancel != nil {
			decodeCancel()
			decodeCancel = nil
		}
		if active != nil {
			active.Close()
			active = nil
		}
		count = nil
		syncTick()
		setPower(false)
	}
	cleanup := func() {
		if rootCancel != nil {
			rootCancel()
			rootCancel = nil
		}
		closeOutput()
		if spool != nil {
			s := spool
			spool = nil
			s.Close()
		}
	}
	var root context.Context
	prepare := func(seek float64) {
		gen++
		g := gen
		closeOutput()
		offset = seek
		ctx, cancel := context.WithCancel(root)
		decodeCancel = cancel
		current := spool
		state.Loading = true
		state.Playing = false
		state.Ended = false
		state.Error = ""
		state.DeviceError = false
		p.set(state)
		p.workers.Add(1)
		go func() {
			defer p.workers.Done()
			result := ready{gen: g, offset: seek}
			var source io.Reader = current.Reader(ctx)
			seekable := current.SeekableReader(ctx)
			if seek > 0 && seekable != nil {
				source = seekable
			}
			dec, err := newPCMDecoder(source)
			if err == nil {
				skip := int64(seek * float64(dec.SampleRate()) * 4)
				skip -= skip % 4
				if skip > 0 {
					if seekable != nil {
						if dec.Length() >= 4 && skip >= dec.Length() {
							skip = dec.Length() - 4
							result.offset = float64(skip) / float64(dec.SampleRate()*4)
						}
						_, err = dec.Seek(skip, io.SeekStart)
					} else {
						_, err = io.CopyN(io.Discard, dec, skip)
					}
				}
			}
			if err == nil {
				err = ctx.Err()
			}
			if err == nil {
				var out *oto.Context
				out, err = device()
				result.deviceError = err != nil
				if err == nil {
					result.output = out
					result.counter = &counter{r: newResampler(dec, dec.SampleRate())}
					result.p = out.NewPlayer(result.counter)
					result.p.SetBufferSize(Rate * 4 / 4)
				}
			}
			result.err = err
			select {
			case p.results <- result:
			case <-p.done:
				if result.p != nil {
					result.p.Close()
				}
				if runtime.GOOS == "android" && result.output != nil {
					_ = result.output.Suspend()
				}
			}
		}()
	}
	// URL resolution is cancellable and happens outside the command loop.
	type resolved struct {
		gen uint64
		s   *Spool
		err error
	}
	resolvedCh := make(chan resolved)
	for {
		select {
		case c := <-p.commands:
			switch c.kind {
			case "play":
				gen++
				cleanup()
				paused = c.paused
				offset = c.seconds
				state = Snapshot{Loading: true, ID: c.id, Position: offset, Paused: paused}
				p.set(state)
				root, rootCancel = trackContext()
				ctx := root
				g := gen
				p.workers.Add(1)
				go func() {
					defer p.workers.Done()
					var raw string
					var err error
					var s *Spool
					if c.filePath != "" {
						if c.fileSHA256 != "" {
							s, err = OpenVerifiedLocal(ctx, c.filePath, c.fileSHA256)
						} else {
							s, err = OpenLocal(c.filePath)
						}
					} else {
						raw, err = c.url(ctx)
						if err == nil {
							s, err = OpenStream(ctx, raw, p.dir)
						}
					}
					select {
					case resolvedCh <- resolved{g, s, err}:
					case <-p.done:
						if s != nil {
							s.Close()
						}
					}
				}()
			case "seek":
				if spool != nil {
					prepare(math.Max(0, c.seconds))
				}
			case "toggle", "pause", "resume":
				if c.kind == "toggle" {
					paused = !paused
				} else {
					paused = c.kind == "pause"
				}
				if active != nil {
					if paused {
						active.Pause()
					} else {
						active.Play()
					}
				}
				state.Paused = paused
				state.Playing = active != nil && !paused
				setPower(active != nil && !paused)
				syncTick()
				p.set(state)
			case "volume":
				volume = math.Max(0, math.Min(1, c.volume))
				if active != nil {
					active.SetVolume(volume * volume)
				}
			case "stop":
				gen++
				cleanup()
				state = Snapshot{}
				p.set(state)
			case "close":
				gen++
				cleanup()
				return
			}
		case r := <-resolvedCh:
			if r.gen != gen {
				if r.s != nil {
					r.s.Close()
				}
				continue
			}
			if r.err != nil {
				cleanup()
				state.Loading, state.Playing = false, false
				state.Error = "Не удалось загрузить трек: " + r.err.Error()
				p.set(state)
				continue
			}
			spool = r.s
			prepare(offset)
		case r := <-p.results:
			if r.output != nil && power.output == nil {
				power.output, power.running = r.output, true
			}
			if r.gen != gen {
				if r.p != nil {
					r.p.Close()
				}
				setPower(active != nil && !paused)
				continue
			}
			if r.err != nil {
				state.Loading = false
				if errors.Is(r.err, io.EOF) {
					state.Ended = true
					state.Playing = false
					closeOutput()
					p.set(state)
					continue
				}
				state.Error = "Не удалось начать воспроизведение: " + r.err.Error()
				state.DeviceError = r.deviceError
				if errors.Is(r.err, context.Canceled) {
					continue
				}
				cleanup()
				p.set(state)
				continue
			}
			active = r.p
			count = r.counter
			offset = r.offset
			active.SetVolume(volume * volume)
			if !paused {
				active.Play()
			}
			state.Loading = false
			state.Paused = paused
			state.Playing = !paused
			state.Position = offset
			setPower(!paused)
			syncTick()
			p.set(state)
		case <-tickC:
			if active != nil && count != nil {
				state.Position = offset + math.Max(0, float64(count.n.Load()-int64(active.BufferedSize()))/(Rate*4))
				state.Playing = active.IsPlaying()
				if !paused && count.eof.Load() && !state.Playing {
					state.Ended = true
				}
				if err := output.Err(); err != nil {
					state.Error = fmt.Sprintf("Ошибка аудиоустройства: %v", err)
					state.DeviceError = true
					state.Playing = false
				} else if count.failed.Load() || active.Err() != nil {
					state.Error = "Воспроизведение прервано"
					state.Playing = false
				}
				if spool != nil {
					state.Downloaded, _, _ = spool.Status()
				}
				if state.Error != "" {
					cleanup()
				} else if state.Ended {
					closeOutput()
				}
				p.set(state)
			}
		}
	}
}

// Linear stereo resampling keeps one output context for all MP3 sample rates.
type resampler struct {
	src              *bufio.Reader
	step, phase      float64
	a, b             [2]float64
	initialized      bool
	last             bool
	frameBytes       [4]byte
	tail             [4]byte
	tailPos, tailLen int
}

func newResampler(r io.Reader, rate int) io.Reader {
	if rate == Rate {
		return r
	}
	return &resampler{src: bufio.NewReaderSize(r, 16<<10), step: float64(rate) / Rate}
}
func (r *resampler) frame() (v [2]float64, e error) {
	_, e = io.ReadFull(r.src, r.frameBytes[:])
	v[0] = float64(int16(binary.LittleEndian.Uint16(r.frameBytes[:2])))
	v[1] = float64(int16(binary.LittleEndian.Uint16(r.frameBytes[2:])))
	return
}
func (r *resampler) Read(p []byte) (int, error) {
	n := 0
	for len(p) > 0 {
		if r.tailPos < r.tailLen {
			k := copy(p, r.tail[r.tailPos:r.tailLen])
			r.tailPos += k
			p = p[k:]
			n += k
			continue
		}
		if !r.initialized {
			var e error
			r.a, e = r.frame()
			if e != nil {
				if n > 0 {
					return n, nil
				}
				return 0, io.EOF
			}
			r.b, e = r.frame()
			if e != nil {
				r.b = r.a
				r.last = true
			}
			r.initialized = true
		}
		for r.phase >= 1 {
			if r.last {
				if n > 0 {
					return n, nil
				}
				return 0, io.EOF
			}
			r.a = r.b
			var e error
			r.b, e = r.frame()
			if e != nil {
				r.b = r.a
				r.last = true
			}
			r.phase--
		}
		for ch := 0; ch < 2; ch++ {
			v := int16(r.a[ch] + (r.b[ch]-r.a[ch])*r.phase)
			binary.LittleEndian.PutUint16(r.tail[ch*2:], uint16(v))
		}
		r.phase += r.step
		r.tailPos, r.tailLen = 0, 4
	}
	return n, nil
}
