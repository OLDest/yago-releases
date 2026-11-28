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

func (u *UI) trackActionsButton(gtx layout.Context) layout.Dimensions {
	d := u.fitButton(gtx, &u.disc.actions, "Трек...", u.disc.menu == "actions")
	if u.disc.menu != "actions" {
		return d
	}
	return u.menuDropdown(gtx, d, false)
}

// Both selectors use one small, contrasting popup and the same outside-click shield.
func (u *UI) menuDropdown(gtx layout.Context, d layout.Dimensions, below bool) layout.Dimensions {
	width := min(gtx.Dp(320), u.fit.viewport.X-gtx.Dp(16))
	if (u.disc.menu == "services" || u.disc.menu == "accounts") && u.fit.viewport.X < gtx.Dp(900) {
		// This anchor sits before the update button, not at the screen edge.
		// A full 320dp popup extends past the left edge on narrow phones.
		width = min(gtx.Dp(220), u.fit.viewport.X*2/5)
	}
	height := max(gtx.Dp(100), u.fit.viewport.Y-gtx.Dp(120))
	row, padding, pager := gtx.Dp(40), gtx.Dp(12), gtx.Dp(36)
	size := max(1, (height-padding)/row)
	if size < len(u.disc.entries) {
		size = max(1, (height-padding-pager)/row)
	}
	pages := max(1, (len(u.disc.entries)+size-1)/size)
	u.fit.menuPage = min(u.fit.menuPage, pages-1)
	u.fit.menuSize = size
	start, end := u.fit.menuPage*size, min(len(u.disc.entries), (u.fit.menuPage+1)*size)
	popupHeight := (end-start)*row + padding
	if pages > 1 {
		popupHeight += pager
	}
	macro := op.Record(gtx.Ops)
	shield := gtx
	shield.Constraints = layout.Exact(image.Pt(20000, 20000))
	offset := op.Offset(image.Pt(-10000, -10000)).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, color.NRGBA{A: 105}, clip.Rect{Max: shield.Constraints.Min}.Op())
	u.disc.dismiss.Layout(shield, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} })
	offset.Pop()
	x := 0
	if u.disc.menu != "waves" && (below || u.fit.viewport.X < gtx.Dp(900)) {
		x = d.Size.X - width
	}
	y := -popupHeight - gtx.Dp(6)
	if below {
		y = d.Size.Y + gtx.Dp(6)
	}
	position := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	radius, edge, shadow := gtx.Dp(10), gtx.Dp(1), gtx.Dp(3)
	paint.FillShape(gtx.Ops, color.NRGBA{A: 150}, clip.UniformRRect(image.Rect(-shadow, -shadow, width+shadow, popupHeight+shadow*2), radius+shadow).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, accent, clip.UniformRRect(image.Rect(0, 0, width, popupHeight), radius).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 37, G: 42, B: 52, A: 255}, clip.UniformRRect(image.Rect(edge, edge, width-edge, popupHeight-edge), radius-edge).Op(gtx.Ops))
	for i := start; i < end; i++ {
		at := op.Offset(image.Pt(gtx.Dp(6), gtx.Dp(6)+(i-start)*row)).Push(gtx.Ops)
		cell := gtx
		cell.Constraints = layout.Exact(image.Pt(width-padding, row))
		if u.disc.entries[i].Selected {
			paint.FillShape(gtx.Ops, accent, clip.UniformRRect(image.Rect(0, 0, width-padding, row), gtx.Dp(6)).Op(gtx.Ops))
		}
		u.disc.clicks[i].Layout(cell, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 5, Bottom: 5, Left: 7, Right: 7}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Label(u.textTheme(u.disc.entries[i].Label), 12, u.disc.entries[i].Label)
				if u.disc.entries[i].Selected {
					l.Color = bg
				}
				l.MaxLines = 2
				return l.Layout(gtx)
			})
		})
		at.Pop()
	}
	if pages > 1 {
		at := op.Offset(image.Pt(gtx.Dp(6), popupHeight-pager-gtx.Dp(6))).Push(gtx.Ops)
		p := gtx
		p.Constraints = layout.Exact(image.Pt(width-padding, pager))
		layout.Flex{Alignment: layout.Middle}.Layout(p, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.fit.menuPage == 0 {
				return layout.Dimensions{}
			}
			return u.fitButton(gtx, &u.fit.menuPrevious, "<", false)
		}), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, fmt.Sprintf("%d / %d", u.fit.menuPage+1, pages), 11, muted)
			})
		}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.fit.menuPage+1 == pages {
				return layout.Dimensions{}
			}
			return u.fitButton(gtx, &u.fit.menuNext, ">", false)
		}))
		at.Pop()
	}
	position.Pop()
	op.Defer(gtx.Ops, macro.Stop())
	return d
}
