package audio

import (
	"errors"
	"sync/atomic"
	"testing"
)

type fakeOutput struct {
	suspend, resume int
	err             error
}

func (p *fakeOutput) Suspend() error { p.suspend++; return p.err }
func (p *fakeOutput) Resume() error  { p.resume++; return p.err }

func TestOutputPowerSuspendsOnceAndResumesOnDemand(t *testing.T) {
	f := &fakeOutput{}
	p := outputPower{output: f, running: true}
	for i := 0; i < 100; i++ {
		if err := p.set(false); err != nil {
			t.Fatal(err)
		}
	}
	if f.suspend != 1 || f.resume != 0 {
		t.Fatal("idle device still being controlled", f)
	}
	if err := p.set(true); err != nil {
		t.Fatal(err)
	}
	if err := p.set(true); err != nil {
		t.Fatal(err)
	}
	if f.resume != 1 {
		t.Fatal("device did not resume exactly once", f)
	}
	f.err = errors.New("device failed")
	if p.set(false) == nil || !p.running {
		t.Fatal("failed suspend lost device state")
	}
}

func TestStateChangesDoNotWakeControllerForPositionOnly(t *testing.T) {
	var redraws atomic.Int32
	p := &Player{changes: make(chan struct{}, 1), notify: func() { redraws.Add(1) }}
	s := Snapshot{ID: 1, Playing: true}
	p.set(s)
	<-p.Changes()
	s.Position = 1
	p.set(s)
	select {
	case <-p.Changes():
		t.Fatal("position woke background controller")
	default:
	}
	s.Paused = true
	s.Playing = false
	p.set(s)
	<-p.Changes()
	n := redraws.Load()
	for i := 0; i < 100; i++ {
		p.set(s)
	}
	if redraws.Load() != n {
		t.Fatal("identical paused snapshots redraw")
	}
}
