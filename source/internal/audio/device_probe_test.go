package audio

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestDeviceProbe(t *testing.T) {
	if os.Getenv("YAGO_DEVICE_PROBE") == "" {
		t.Skip("opt-in device probe")
	}
	out, err := device()
	if err == nil {
		t.Logf("working device: %v", out.Err())
		t.Skip("failure path needs unavailable audio device")
	}
	t.Logf("driver failure: %v", err)
	fixture := os.Getenv("YAGO_TEST_MP3")
	if fixture == "" {
		t.Fatal("set YAGO_TEST_MP3 for driver failure propagation check")
	}
	p := New(t.TempDir(), func() {})
	defer p.Close()
	p.PlayFileAt(fixture, 0, false)
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		s := p.Snapshot()
		if s.Error != "" {
			if s.Playing || s.Loading || !strings.Contains(s.Error, err.Error()) {
				t.Fatalf("driver error lost: %+v", s)
			}
			t.Logf("reported: %s", s.Error)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("driver failure was hidden")
}
