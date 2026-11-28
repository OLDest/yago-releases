package main

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
	"testing"
	"time"
)

func TestWaveDropdownStaysOnSettingsAndSavesChoice(t *testing.T) {
	u := serviceTestUI(t)
	u.services.mode = "zvuk"
	u.applyServiceMode(false)
	options, e := u.services.zvuk.WaveOptions(u.ctx)
	if e != nil {
		t.Fatal(e)
	}
	u.optionsMenu(options)
	var router input.Router
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1080, 740))}
	frame := func() { ops.Reset(); u.layout(gtx); router.Frame(&ops) }
	click := func(x, y float32) {
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, y)})
		u.discoveryInputs(gtx)
	}
	frame()
	click(100, 125)
	if u.disc.menu != "wave-options" || u.disc.waveOpenGroup != "popular" {
		t.Fatal("group changed page or did not open", u.disc.menu, u.disc.waveOpenGroup)
	}
	frame()
	click(100, 175)
	if u.disc.menu != "wave-options" || u.disc.waveOpenGroup != "" || u.disc.prefs.ZvukSeeds["popular"] != "zvuk:popular:0" {
		t.Fatal("dropdown choice did not persist on same page", u.disc.menu, u.disc.prefs.ZvukSeeds)
	}
	frame()
	click(100, 125)
	frame()
	click(950, 350)
	if u.disc.menu != "wave-options" || u.disc.waveOpenGroup != "" {
		t.Fatal("outside click closed settings")
	}
}
