package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"yago/internal/audio"
	"yago/internal/music"
)

func TestVolumeMuteAndClamp(t *testing.T) {
	u := &UI{volume: .65, player: audio.New(t.TempDir(), func() {})}
	defer u.player.Close()
	u.muteVolume()
	if !u.muted || u.volume != .65 || u.volumeLabel() != "MUTE" {
		t.Fatal("mute lost level")
	}
	u.muteVolume()
	if u.muted || u.volume != .65 {
		t.Fatal("unmute failed")
	}
	u.changeVolume(2)
	if u.volume != 1 {
		t.Fatal("volume above one")
	}
	u.changeVolume(-3)
	if u.volume != 0 {
		t.Fatal("negative volume")
	}
	u.muteVolume()
	u.changeVolume(.1)
	if u.muted {
		t.Fatal("volume key must unmute")
	}
}
func TestStaleBitrateIgnored(t *testing.T) {
	u := &UI{streamVersion: 3}
	u.handle(event{kind: "stream", streamID: 2, bitrate: 320})
	if u.streamBitrate != 0 {
		t.Fatal("stale track bitrate")
	}
	u.handle(event{kind: "stream", streamID: 3, bitrate: 128})
	if u.streamBitrate != 128 {
		t.Fatal("current track ignored")
	}
}
func TestQualityCycle(t *testing.T) {
	choices := []music.Quality{music.Normal, music.High}
	if music.NextAvailable(music.High, choices) != music.Normal || music.NextAvailable(music.Normal, choices) != music.High {
		t.Fatal("cycle ignored server choices")
	}
	if music.NextAvailable(music.High, []music.Quality{music.High}) != music.High {
		t.Fatal("unsupported mode offered")
	}
}

func TestOld128PreferenceMigrates(t *testing.T) {
	old := stateDir
	stateDir = t.TempDir()
	defer func() { stateDir = old }()
	if e := os.WriteFile(filepath.Join(stateDir, "quality"), []byte("128"), 0600); e != nil {
		t.Fatal(e)
	}
	u := &UI{}
	u.initPlaybackSettings()
	if u.quality != music.High {
		t.Fatal("unverified 128 preference restored")
	}
	u.selectQuality(music.High)
	if u.quality != music.High {
		t.Fatal("unknown qualities offered")
	}
}
func TestOnlyRequiredFontsLoaded(t *testing.T) {
	if got := len(uiFonts()); got != 2 {
		t.Fatalf("expected Regular and Bold UI font faces, got %d", got)
	}
}

func TestBasicSelectionFailureKeepsCurrentStream(t *testing.T) {
	oldDemo := demo
	demo = false
	t.Cleanup(func() { demo = oldDemo })
	url, _ := playbackFixture(t)
	u := &UI{ctx: context.Background(), events: make(chan event, 4), quality: music.High, index: 0, queue: []music.Track{{ID: "1", DurationMS: 60000}}, current: music.Track{ID: "1", DurationMS: 60000}}
	u.player = audio.New(t.TempDir(), func() {})
	t.Cleanup(u.player.Close)
	u.player.Volume(0)
	u.playID = u.player.Play(func(context.Context) (string, error) { return url, nil })
	audioReady(t, u.player, func(s audio.Snapshot) bool { return s.Playing })
	before := u.playID
	u.api = music.New("")
	u.api.HTTP.Transport = modeTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{\"result\":[{\"codec\":\"mp3\",\"bitrateInKbps\":320,\"downloadInfoUrl\":\"https://storage.yandex.net/320\"}]}"))}, nil
	})
	u.selectQuality(music.Normal)
	select {
	case e := <-u.events:
		u.handle(e)
	case <-time.After(3 * time.Second):
		t.Fatal("quality preflight did not finish")
	}
	s := u.player.Snapshot()
	if u.quality != music.High || s.ID != before || !s.Playing || u.qualitySwitching {
		t.Fatal("failed selection interrupted old playback", s)
	}
}

type modeTestTransport func(*http.Request) (*http.Response, error)

func (f modeTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestBasicPreferenceRestored(t *testing.T) {
	old := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = old })
	u := &UI{}
	u.selectQuality(music.Normal)
	v := &UI{}
	v.initPlaybackSettings()
	if v.quality != music.Normal {
		t.Fatal("Basic preference lost")
	}
	u.selectQuality(music.High)
	v.initPlaybackSettings()
	if v.quality != music.High {
		t.Fatal("HQ preference lost")
	}
}
