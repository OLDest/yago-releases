package main

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
	"testing"
)

func TestMenuPagesFitAndReachEveryEntry(t *testing.T) {
	u := &UI{theme: material.NewTheme()}
	u.theme.Shaper = text.NewShaper(text.WithCollection(uiFonts()))
	entries := make([]menuEntry, 80)
	for i := range entries {
		entries[i].Label = "Настройка волны"
	}
	u.showMenu("test", entries)
	for _, size := range []image.Point{image.Pt(304, 220), image.Pt(780, 250), image.Pt(1584, 600)} {
		seen := make([]bool, len(entries))
		u.fit.menuPage = 0
		for {
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
			d := u.menuGrid(gtx)
			if d.Size.X > size.X || d.Size.Y > size.Y || u.fit.menuSize <= 0 {
				t.Fatal("menu overflow", size, d.Size)
			}
			start := u.fit.menuPage * u.fit.menuSize
			end := min(len(entries), start+u.fit.menuSize)
			for i := start; i < end; i++ {
				seen[i] = true
			}
			if end == len(entries) {
				break
			}
			u.fit.menuPage++
		}
		for i, ok := range seen {
			if !ok {
				t.Fatal("unreachable menu entry", size, i)
			}
		}
	}
}
func TestVisibleRowsStayInsideViewport(t *testing.T) {
	u := &UI{theme: material.NewTheme()}
	u.theme.Shaper = text.NewShaper(text.WithCollection(uiFonts()))
	for _, h := range []int{110, 180, 400} {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(320, h))}
		count := 0
		u.fitRows(gtx, "test", 2000, 62, func(gtx layout.Context, i int) layout.Dimensions {
			count++
			return layout.Dimensions{Size: image.Pt(320, 62)}
		})
		if count != fitCapacity(h, 62, 38) || count*62+38 > h {
			t.Fatal("rows clipped", h, count)
		}
	}
}
