package mobile

import (
	"encoding/json"
	"sync"
	"time"
)

// Commands arrive from MediaSession, notification buttons and headset keys.
var Commands = make(chan string, 32)

type Playback struct {
	TrackID         string `json:"trackId"`
	Liked           bool   `json:"liked"`
	FeedbackEnabled bool   `json:"feedbackEnabled"`
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	Duration        int64  `json:"duration"`
	Position        int64  `json:"position"`
	Playing         bool   `json:"playing"`
	Loading         bool   `json:"loading"`
	Active          bool   `json:"active"`
}

func Publish(p Playback) error {
	publication.Lock()
	defer publication.Unlock()
	now := time.Now()
	if !publication.due(p, now) {
		return nil
	}
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	_, e = Call("publish", string(b))
	if e == nil {
		publication.last, publication.at, publication.sent = p, now, true
	}
	return e
}

type publicationState struct {
	sync.Mutex
	last Playback
	at   time.Time
	sent bool
}

var publication publicationState

// Android interpolates position from PlaybackState's timestamp and speed.
// Send semantic changes immediately; correct playing position every 15s,
// and seeks immediately. Identical paused/stopped state needs no JNI or IPC.
func (s *publicationState) due(p Playback, now time.Time) bool {
	if !s.sent {
		return true
	}
	a, b := s.last, p
	a.Position, b.Position = 0, 0
	if a != b {
		return true
	}
	if !p.Active {
		return false
	}
	delta := p.Position - s.last.Position
	if delta < 0 {
		return true
	}
	if !p.Playing {
		return delta != 0
	}
	elapsed := now.Sub(s.at)
	drift := delta - elapsed.Milliseconds()
	if drift > 2000 || drift < -2000 {
		return true
	}
	return elapsed >= 15*time.Second
}
func OpenURL(raw string) error { _, e := Call("openURL", raw); return e }
