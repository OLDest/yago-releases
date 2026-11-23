package main

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
	"image/color"
)

func (u *UI) trackRow(gtx layout.Context, i int) layout.Dimensions {
	return fixedHeight(62, func(gtx layout.Context) layout.Dimensions {
		selected := u.page != "playlists" && u.tracks[i].ID == u.current.ID
		col := panel
		if selected {
			col = color.NRGBA{R: 47, G: 45, B: 33, A: 255}
		}
		paint.FillShape(gtx.Ops, col, clip.RRect{Rect: image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Min.Y-4)}, NE: 10, NW: 10, SE: 10, SW: 10}.Op(gtx.Ops))
		return layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Playback and the action button are siblings with disjoint hit areas.
			// A nested Clickable can deliver the same pointer click to its parent.
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return u.rows[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								n := fmt.Sprintf("%02d", i+1)
								if selected {
									return playMarker(gtx)
								}
								return u.label(gtx, n, 13, accent)
							}), gap(16),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								title, artist := "", ""
								if u.page == "playlists" {
									title = u.playlists[i].Title
									artist = fmt.Sprintf("%d треков", u.playlists[i].TrackCount)
									artist += u.sourceSuffix(string(u.playlists[i].Kind))
								} else {
									title = u.tracks[i].Title
									artist = u.tracks[i].Artist() + u.sourceSuffix(string(u.tracks[i].ID))
								}
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										l := material.Body1(u.textTheme(title), title)
										l.TextSize = unit.Sp(14)
										l.MaxLines = 1
										return l.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										l := material.Caption(u.textTheme(artist), artist)
										l.Color = muted
										l.MaxLines = 1
										return l.Layout(gtx)
									}))
							}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if u.page == "playlists" {
									return layout.Dimensions{}
								}
								return u.label(gtx, duration(float64(u.tracks[i].DurationMS)/1000), 12, muted)
							}))
					})
				}), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.trackRowActions(gtx, i) }))
		})
	})(gtx)
}
