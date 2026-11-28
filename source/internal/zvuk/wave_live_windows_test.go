package zvuk

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"yago/internal/store"
)

func TestLivePersonalWave(t *testing.T) {
	state := os.Getenv("YAGO_ZVUK_LIVE_STATE")
	if state == "" {
		t.Skip("explicit account check")
	}
	device, e := os.ReadFile(filepath.Join(state, "device-id"))
	if e != nil || len(device) != 32 {
		t.Fatal("device unavailable")
	}
	token, e := store.Load(string(device) + ":zvuk")
	if e != nil {
		t.Fatal("saved account unavailable")
	}
	c := New(token.Access)
	defer c.Close()
	c.AccountHint = token.Profile
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var ids []string
	session := ""
	for n := 0; n < 8 && len(ids) < 20; n++ {
		w, e := c.WaveWithSeeds(ctx, session, ids, []string{"zvuk:personal"})
		if e != nil {
			t.Fatal(e)
		}
		session = w.Session
		for _, s := range w.Sequence {
			ids = append(ids, string(s.Track.ID))
		}
		t.Logf("request %d, unique tracks %d", n+1, len(ids))
	}
	if len(ids) < 20 {
		t.Fatalf("expected initial 5 plus 15 prefetch, got %d", len(ids))
	}
	w, e := c.WaveWithSeeds(ctx, "", nil, []string{"zvuk:energy:1", "zvuk:fun:0", "zvuk:language:russian", "zvuk:popular:1", "zvuk:vocal:0"})
	if e != nil || len(w.Sequence) == 0 {
		t.Fatal("custom wave settings", e)
	}
	t.Logf("custom settings accepted, tracks %d", len(w.Sequence))
}
