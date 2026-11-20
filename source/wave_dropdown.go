package main

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
	"image"
	"image/color"
)

// Only the open group owns popup widgets. No new window, webview or font is needed.
func (u *UI) waveGroupDropdown(gtx layout.Context, d layout.Dimensions, group string, above, below int) layout.Dimensions {
	if u.disc.waveOpenGroup != group {
		return d
	}
	row, pad, pager := gtx.Dp(32), gtx.Dp(8), gtx.Dp(32)
	available := max(above, below) - gtx.Dp(8)
	size := max(1, (available-pad)/row)
	if size < len(u.disc.waveChoices) {
		size = max(1, (available-pad-pager)/row)
	}
	pages := max(1, (len(u.disc.waveChoices)+size-1)/size)
	u.disc.wavePopupPage = min(u.disc.wavePopupPage, pages-1)
	start := u.disc.wavePopupPage * size
	end := min(start+size, len(u.disc.waveChoices))
	width := min(gtx.Dp(340), d.Size.X)
	height := (end-start)*row + pad
	if pages > 1 {
		height += pager
	}
	y := d.Size.Y + gtx.Dp(4)
	if above > below {
		y = -height - gtx.Dp(4)
	}
	macro := op.Record(gtx.Ops)
	at := op.Offset(image.Pt(-10000, -10000)).Push(gtx.Ops)
	shield := gtx
	shield.Constraints = layout.Exact(image.Pt(20000, 20000))
	paint.FillShape(gtx.Ops, color.NRGBA{A: 90}, clip.Rect{Max: shield.Constraints.Max}.Op())
	u.disc.waveDismiss.Layout(shield, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
	at.Pop()
	at = op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	col := u.waveGroupColor(group)
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(image.Rect(0, 0, width, height), gtx.Dp(8)).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 37, G: 42, B: 52, A: 255}, clip.UniformRRect(image.Rect(1, 1, width-1, height-1), gtx.Dp(8)).Op(gtx.Ops))
	for i := start; i < end; i++ {
		pos := op.Offset(image.Pt(gtx.Dp(4), gtx.Dp(4)+(i-start)*row)).Push(gtx.Ops)
		cell := gtx
		cell.Constraints = layout.Exact(image.Pt(width-pad, row))
		if u.disc.waveChoices[i].Selected {
			paint.FillShape(gtx.Ops, col, clip.UniformRRect(image.Rectangle{Max: cell.Constraints.Max}, gtx.Dp(5)).Op(gtx.Ops))
		}
		u.disc.waveChoiceClicks[i].Layout(cell, inset(6, func(gtx layout.Context) layout.Dimensions {
			l := material.Label(u.textTheme(u.disc.waveChoices[i].Label), 12, u.disc.waveChoices[i].Label)
			l.MaxLines = 1
			if u.disc.waveChoices[i].Selected {
				l.Color = bg
			}
			return l.Layout(gtx)
		}))
		pos.Pop()
	}
	if pages > 1 {
		pos := op.Offset(image.Pt(gtx.Dp(4), height-pager-gtx.Dp(4))).Push(gtx.Ops)
		cell := gtx
		cell.Constraints = layout.Exact(image.Pt(width-pad, pager))
		layout.Flex{Alignment: layout.Middle}.Layout(cell, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.disc.wavePopupPage == 0 {
				return layout.Dimensions{}
			}
			return u.fitButton(gtx, &u.disc.wavePrev, "<", false)
		}), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, fmt.Sprintf("%d / %d", u.disc.wavePopupPage+1, pages), 11, muted)
			})
		}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.disc.wavePopupPage+1 == pages {
				return layout.Dimensions{}
			}
			return u.fitButton(gtx, &u.disc.waveNext, ">", false)
		}))
		pos.Pop()
	}
	at.Pop()
	op.Defer(gtx.Ops, macro.Stop())
	return d
}
