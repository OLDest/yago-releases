package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"yago/internal/audio"
	"yago/internal/music"
	"yago/internal/store"
)

func waveTracksForTest(start, n int) []music.Track {
	v := make([]music.Track, n)
	for i := range v {
		v[i] = music.Track{ID: music.ID(fmt.Sprint(start + i)), DurationMS: 180000}
	}
	return v
}
func TestWavePrefetchGets15WithoutRestartingOrBlockingNext(t *testing.T) {
	oldDemo, oldURL := demo, demoURL
	demo, demoURL = true, ""
	defer func() { demo, demoURL = oldDemo, oldURL }()
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "feedback") {
			w.WriteHeader(204)
			return
		}
		batch := calls.Add(1)
		if batch == 1 {
			close(entered)
			<-release
		}
		var b struct{ Queue []string }
		json.NewDecoder(r.Body).Decode(&b)
		if len(b.Queue) < 5+int(batch-1)*5 {
			t.Error("missing exclusions", b.Queue)
		}
		fmt.Fprintf(w, `{"radioSessionId":"session","batchId":"batch-%d","sequence":[`, batch)
		for i := 0; i < 5; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"track":{"id":"%d"}}`, 100+int(batch-1)*5+i)
		}
		fmt.Fprint(w, `]}`)
	}))
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := music.New("")
	a.Base = s.URL
	p := audio.New(t.TempDir(), nil)
	defer p.Close()
	u := &UI{ctx: ctx, api: a, player: p, events: make(chan event, 10), isWave: true, queue: waveTracksForTest(0, 5), current: music.Track{ID: "0"}, index: 0, playID: 17, streamVersion: 9, requestedOffset: 42, session: "session", batch: "initial"}
	u.resetOrder(0)
	u.waveContexts = waveContexts(5, "session", "initial")
	// Opening account login must not cancel or downgrade this Wave queue.
	loginCtx, closeLogin := u.prepareProfileLogin()
	defer closeLogin()
	if loginCtx.Err() != nil || !u.isWave {
		t.Fatal("login changed Wave")
	}
	u.maybePrefetchWave()
	u.maybePrefetchWave()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("prefetch did not start on first track")
	}
	u.advance(false)
	if u.current.ID != "1" || u.waveNextCancel == nil {
		t.Fatal("background refill blocked Next or was canceled")
	}
	version, playID := u.streamVersion, u.playID
	close(release)
	select {
	case e := <-u.events:
		u.handle(e)
	case <-time.After(time.Second):
		t.Fatal("refill timeout")
	}
	if calls.Load() != 3 || len(u.queue) != 20 || u.current.ID != "1" || u.index != 1 || u.playID != playID || u.streamVersion != version {
		t.Fatal("append interrupted playback", calls.Load(), len(u.queue), u.current.ID)
	}
	for i := 0; i < 5; i++ {
		if u.queue[i].ID != music.ID(fmt.Sprint(i)) {
			t.Fatal("original order changed")
		}
	}
	if u.batch != "initial" || u.waveContexts[10].Batch != "batch-2" || u.waveContexts[15].Batch != "batch-3" {
		t.Fatal("batch feedback metadata lost")
	}
	u.startAt(10, 0, false, false)
	if u.batch != "batch-2" {
		t.Fatal("selected track uses wrong batch")
	}
}
func TestWaveRefillThresholdAndHistoryBound(t *testing.T) {
	u := &UI{isWave: true, queue: waveTracksForTest(0, 20), index: 0, current: music.Track{ID: "0"}, playID: 13, streamVersion: 7}
	u.resetOrder(0)
	u.waveContexts = waveContexts(20, "session", "initial")
	u.orderPos = 14
	if u.upcomingWave() != 5 {
		t.Fatal("threshold should exclude current")
	}
	u.orderPos = 15
	if u.upcomingWave() != 4 {
		t.Fatal("low water mismatch")
	}
	for cycle := 0; cycle < 300; cycle++ {
		u.orderPos = len(u.order) - 5
		u.index = u.order[u.orderPos]
		u.current = u.queue[u.index]
		id := u.current.ID
		u.waveContinuationReady(event{waveID: u.waveNextID, tracks: waveTracksForTest(1000+cycle*15, 15), waveContexts: waveContexts(15, "session", "new")})
		if len(u.queue) > 25 || len(u.waveContexts) != len(u.queue) || u.queue[u.index].ID != id || u.playID != 13 || u.streamVersion != 7 || u.upcomingWave() != 19 {
			t.Fatal("queue grew or current moved", cycle, len(u.queue), u.upcomingWave())
		}
		if len(u.order) != len(u.queue) {
			t.Fatal("invalid order")
		}
	}
}
func TestWaveRefillFiltersDuplicatesAndBlockedArtists(t *testing.T) {
	var requests atomic.Int32
	a := music.New("")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		fmt.Fprintf(w, `{"radioSessionId":"s","batchId":"b","sequence":[{"track":{"id":"existing"}},{"track":{"id":"blocked-%d","artists":[{"id":"bad"}]}},{"track":{"id":"good-%d"}}]}`, n, n)
	}))
	defer s.Close()
	a.Base = s.URL
	tracks, contexts, e := fetchWaveTail(context.Background(), a, "s", []string{"existing"}, nil, store.Preferences{Artists: map[music.ID]string{"bad": "Bad"}})
	if len(tracks) != waveMaxRequests || len(contexts) != len(tracks) || e == nil || requests.Load() != waveMaxRequests {
		t.Fatal("bounded partial refill", len(tracks), e)
	}
	for _, x := range tracks {
		if !strings.HasPrefix(string(x.ID), "good-") {
			t.Fatal("blocked/duplicate appended", x)
		}
	}
}
func TestWavePrefetchErrorDoesNotStopPlayingAndBacksOff(t *testing.T) {
	u := &UI{isWave: true, queue: waveTracksForTest(0, 5), current: music.Track{ID: "0"}, index: 0, playID: 9, streamVersion: 7}
	u.resetOrder(0)
	u.waveContinuationReady(event{waveID: 0, err: fmt.Errorf("offline")})
	if u.advancePending || u.stopped || u.playID != 9 || u.streamVersion != 7 || !u.waveRetryAt.After(time.Now()) {
		t.Fatal("error interrupted playback")
	}
}
func TestWavePrefetchPreservesRefreshedViewAndShuffle(t *testing.T) {
	u := &UI{isWave: true, queue: waveTracksForTest(0, 10), index: 3, current: music.Track{ID: "3"}, shuffled: true, waveViewRefreshed: true, waveTracks: []music.Track{{ID: "fresh"}}, session: "old"}
	u.resetOrder(3)
	old := append([]int(nil), u.order...)
	u.waveContinuationReady(event{waveID: 0, tracks: waveTracksForTest(100, 15), waveContexts: waveContexts(15, "old", "new-batch")})
	if len(u.queue) != 25 || u.waveTracks[0].ID != "fresh" || u.current.ID != "3" {
		t.Fatal("prefetch took over refreshed view")
	}
	for i, x := range old {
		if u.order[i] != x {
			t.Fatal("shuffle order changed")
		}
	}
}

func TestStartingFirstWaveTrackAutomaticallyPrefetches(t *testing.T) {
	oldDemo := demo
	demo = false
	defer func() { demo = oldDemo }()
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "feedback") {
			w.WriteHeader(204)
			return
		}
		if strings.Contains(r.URL.Path, "download-info") || strings.Contains(r.URL.Path, "get-file-info") {
			http.Error(w, "no audio in this metadata test", 401)
			return
		}
		n := calls.Add(1)
		fmt.Fprint(w, `{"radioSessionId":"s","batchId":"next","sequence":[`)
		for i := 0; i < 5; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"track":{"id":"next-%d"}}`, int(n)*5+i)
		}
		fmt.Fprint(w, `]}`)
	}))
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := audio.New(t.TempDir(), nil)
	defer p.Close()
	a := music.New("")
	a.Base = s.URL
	u := &UI{ctx: ctx, api: a, player: p, events: make(chan event, 8)}
	u.handle(event{kind: "wave", tracks: waveTracksForTest(0, 5), session: "s", batch: "first"})
	playID := u.playID
	select {
	case e := <-u.events:
		u.handle(e)
	case <-time.After(2 * time.Second):
		t.Fatal("no background refill after first start")
	}
	if calls.Load() != 3 || len(u.queue) != 20 || u.current.ID != "0" || u.playID != playID || u.index != 0 {
		t.Fatal("first track refill changed playback", calls.Load(), len(u.queue))
	}
}
func TestWaveQueueHardLimitSurvivesShuffleChanges(t *testing.T) {
	u := &UI{isWave: true, queue: waveTracksForTest(0, 20), current: music.Track{ID: "0"}, shuffled: true}
	u.waveContexts = waveContexts(20, "s", "b")
	for n := 0; n < 50; n++ {
		u.resetOrder(0)
		u.waveContinuationReady(event{waveID: u.waveNextID, tracks: waveTracksForTest(1000+n*15, 15), waveContexts: waveContexts(15, "s", "b")})
		if len(u.queue) > waveQueueLimit || u.queue[u.index].ID != "0" {
			t.Fatal("queue cap lost", len(u.queue))
		}
	}
}
