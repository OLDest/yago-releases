package main

import (
	_ "embed"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"image"
	"sync"
)

// The selected parrot icon, pre-sized at build time. No PNG decoder or large
// master image is loaded by the player. One 32x32 image is reused by all headers.
//
//go:embed assets/brand.rgba
var brandPixels []byte
var brandOnce sync.Once
var brandImage paint.ImageOp

func (u *UI) brand(gtx layout.Context) layout.Dimensions {
	brandOnce.Do(func() {
		if len(brandPixels) != 32*32*4 {
			panic("invalid brand icon")
		}
		brandImage = paint.NewImageOp(&image.NRGBA{Pix: brandPixels, Stride: 32 * 4, Rect: image.Rect(0, 0, 32, 32)})
	})
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(28), gtx.Dp(28)))
		return widget.Image{Src: brandImage, Fit: widget.Contain}.Layout(gtx)
	}), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		l := material.Label(u.textTheme("YaGo"), 24, "YaGo")
		l.Font.Weight = font.Bold
		l.Color = accent
		l.MaxLines = 1
		return l.Layout(gtx)
	}))
}
