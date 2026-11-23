package zvuk

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"yago/internal/store"
)

func TestLiveGenreWave(t *testing.T) {
	state := os.Getenv("YAGO_ZVUK_LIVE_STATE")
	if state == "" {
		t.Skip("explicit genre API check")
	}
	device, e := os.ReadFile(filepath.Join(state, "device-id"))
	if e != nil {
		t.Fatal("device unavailable")
	}
	token, e := store.Load(string(device) + ":zvuk")
	if e != nil {
		t.Fatal("account unavailable")
	}
	c := New(token.Access)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, seed := range []string{"zvuk:genre:rock", "zvuk:genre:chanson", "zvuk:genre:russian_rock"} {
		w, e := c.WaveWithSeeds(ctx, "", nil, []string{seed})
		if e != nil || len(w.Sequence) == 0 {
			t.Fatal(seed, e)
		}
		t.Logf("%s accepted: %d tracks", seed, len(w.Sequence))
	}
}
