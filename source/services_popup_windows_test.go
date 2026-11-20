package main

import (
	"fmt"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Inspect the actual deferred popup pixels, including its translated header
// anchor. A width-only test would miss the left-edge clipping in the report.
func TestPhoneProfilesPopupFitsScreen(t *testing.T) {
	for _, c := range []struct {
		size  image.Point
		scale float32
	}{{image.Pt(320, 640), 1}, {image.Pt(380, 820), 1}, {image.Pt(576, 1280), 1.5}} {
		u := serviceTestUI(t)
		u.services.mode = "all"
		u.applyServiceMode(false)
		u.servicesMenu()
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: c.scale, PxPerSp: c.scale}, Constraints: layout.Exact(c.size)}
		paint.Fill(&ops, bg)
		u.layout(gtx)
		w, e := headless.NewWindow(c.size.X, c.size.Y)
		if e != nil {
			t.Fatal(e)
		}
		if e = w.Frame(&ops); e != nil {
			w.Release()
			t.Fatal(e)
		}
		im := image.NewRGBA(image.Rectangle{Max: c.size})
		e = w.Screenshot(im)
		w.Release()
		if e != nil {
			t.Fatal(e)
		}
		bounds := image.Rectangle{}
		for y := 0; y < c.size.Y; y++ {
			for x := 0; x < c.size.X; x++ {
				p := im.RGBAAt(x, y)
				if p.R == accent.R && p.G == accent.G && p.B == accent.B && p.A == 255 {
					r := image.Rect(x, y, x+1, y+1)
					if bounds.Empty() {
						bounds = r
					} else {
						bounds = bounds.Union(r)
					}
				}
			}
		}
		if bounds.Empty() || bounds.Min.X < gtx.Dp(8) || bounds.Max.X > c.size.X-gtx.Dp(8) || bounds.Dx() > gtx.Dp(220) {
			t.Fatalf("popup clipped/too wide: viewport=%v scale=%v bounds=%v", c.size, c.scale, bounds)
		}
		if u.systemFonts {
			t.Fatal("popup loaded extra fonts")
		}
		if out := os.Getenv("YAGO_POPUP_RENDER_DIR"); out != "" {
			if e = os.MkdirAll(out, 0700); e != nil {
				t.Fatal(e)
			}
			f, e := os.Create(filepath.Join(out, fmt.Sprintf("profiles-%dx%d.png", c.size.X, c.size.Y)))
			if e != nil {
				t.Fatal(e)
			}
			e = png.Encode(f, im)
			f.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
}
