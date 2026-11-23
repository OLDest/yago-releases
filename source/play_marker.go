package main

import (
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
)

// A tiny vector avoids loading a system font for a single play glyph.
func playMarker(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Constrain(image.Pt(gtx.Dp(unit.Dp(12)), gtx.Dp(unit.Dp(16))))
	w, h := float32(size.X), float32(size.Y)
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(w*.15, h*.2))
	p.LineTo(f32.Pt(w*.85, h*.5))
	p.LineTo(f32.Pt(w*.15, h*.8))
	p.Close()
	paint.FillShape(gtx.Ops, accent, clip.Outline{Path: p.End()}.Op())
	return layout.Dimensions{Size: size}
}
