package main

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"image"
	"yago/internal/music"
)

type fitUI struct {
	viewport               image.Point
	compact                bool
	key                    string
	offset, visible, count int
	previous, next         widget.Clickable
	menuPage, menuSize     int
	menuPrevious, menuNext widget.Clickable
	followID               music.ID
	followCapacity         int
}

func fitCapacity(height, row, pager int) int { return max(1, (height-pager)/max(1, row)) }
func (u *UI) fitInputs(gtx layout.Context) {
	for u.fit.previous.Clicked(gtx) {
		u.turnListPage(-1)
	}
	for u.fit.next.Clicked(gtx) {
		u.turnListPage(1)
	}
	for u.fit.menuPrevious.Clicked(gtx) {
		u.fit.menuPage = max(0, u.fit.menuPage-1)
	}
	for u.fit.menuNext.Clicked(gtx) {
		u.fit.menuPage++
	}
}

// Buttons and touch gestures share the exact same list-page navigation.
func (u *UI) turnListPage(direction int) {
	if u.fit.visible <= 0 || u.busy {
		return
	}
	remote := u.disc.view.Kind != "" && !u.isOfflineView()
	if direction < 0 {
		if u.fit.offset > 0 {
			u.fit.offset = max(0, u.fit.offset-u.fit.visible)
		} else if remote && u.disc.view.Page > 0 {
			v := u.disc.view
			v.Page--
			u.loadExplore(v)
		}
	} else if direction > 0 {
		if u.fit.offset+u.fit.visible < u.fit.count {
			u.fit.offset += u.fit.visible
		} else if remote && u.disc.view.More {
			v := u.disc.view
			v.Page++
			u.loadExplore(v)
		}
	}
}
func (u *UI) fitRows(gtx layout.Context, key string, count, rowHeight int, row func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	if u.fit.key != key {
		u.fit.key = key
		u.fit.offset = 0
		u.fit.followID = ""
	}
	remote := u.disc.view.Kind != "" && !u.isOfflineView()
	capacity := max(1, gtx.Constraints.Max.Y/max(1, gtx.Dp(unit.Dp(rowHeight))))
	pager := count > capacity || (remote && (u.disc.view.More || u.disc.view.Page > 0))
	if pager {
		capacity = fitCapacity(gtx.Constraints.Max.Y, gtx.Dp(unit.Dp(rowHeight)), gtx.Dp(38))
	}
	u.fit.visible, u.fit.count = capacity, count
	u.followPlayingPage(capacity, count)
	if u.fit.offset >= count {
		u.fit.offset = max(0, ((count-1)/capacity)*capacity)
	}
	start, end := u.fit.offset, min(count, u.fit.offset+capacity)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, end-start)
		for i := start; i < end; i++ {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return row(gtx, i) }))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if !pager {
			return layout.Dimensions{}
		}
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if start == 0 && (!remote || u.disc.view.Page == 0) {
					return layout.Dimensions{}
				}
				return u.fitButton(gtx, &u.fit.previous, "<", false)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				text := fmt.Sprintf("%d-%d / %d", min(start+1, count), end, count)
				if remote {
					text = fmt.Sprintf("%d · %s", u.disc.view.Page+1, text)
				}
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return u.label(gtx, text, 11, muted) })
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if end >= count && (!remote || !u.disc.view.More) {
					return layout.Dimensions{}
				}
				return u.fitButton(gtx, &u.fit.next, ">", false)
			}))
	}))
}

// Follow track changes, without pulling the user back on every rendered frame.
func (u *UI) followPlayingPage(capacity, count int) {
	if u.fit.followID == u.current.ID && u.fit.followCapacity == capacity {
		return
	}
	u.fit.followID, u.fit.followCapacity = u.current.ID, capacity
	if u.current.ID == "" || capacity <= 0 {
		return
	}
	var tracks []music.Track
	switch u.page {
	case "tracks", "offline-tracks":
		tracks = u.tracks
	case "discovery":
		tracks = u.disc.result.Tracks
	default:
		return
	}
	for i, t := range tracks {
		if i >= count {
			break
		}
		if t.ID == u.current.ID {
			u.fit.offset = (i / capacity) * capacity
			return
		}
	}
}
func (u *UI) fitButton(gtx layout.Context, b *widget.Clickable, label string, primary bool) layout.Dimensions {
	gtx.Constraints.Min.X = max(gtx.Constraints.Min.X, min(gtx.Constraints.Max.X, gtx.Dp(44)))
	v := material.Button(u.textTheme(label), b, label)
	v.TextSize = 12
	v.CornerRadius = 8
	v.Inset = layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}
	if !primary {
		v.Background = panel
		v.Color = white
	}
	return v.Layout(gtx)
}
func (u *UI) menuGrid(gtx layout.Context) layout.Dimensions {
	availableHeight := gtx.Constraints.Max.Y
	cols := max(1, min(3, gtx.Constraints.Max.X/max(1, gtx.Dp(270))))
	rows := max(1, gtx.Constraints.Max.Y/max(1, gtx.Dp(52)))
	if rows*cols < len(u.disc.entries) {
		rows = fitCapacity(gtx.Constraints.Max.Y, gtx.Dp(52), gtx.Dp(38))
	}
	size := rows * cols
	u.fit.menuSize = size
	pages := max(1, (len(u.disc.entries)+size-1)/size)
	u.fit.menuPage = min(u.fit.menuPage, pages-1)
	start, end := u.fit.menuPage*size, min(len(u.disc.entries), (u.fit.menuPage+1)*size)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, (end-start+cols-1)/cols)
		for base := start; base < end; base += cols {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var cells [3]layout.FlexChild
				for c := 0; c < cols; c++ {
					i := base + c
					cells[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if i >= end {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: 3, Bottom: 3, Left: 3, Right: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							d := u.disc.clicks[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								borderColor := muted
								if u.disc.entries[i].Action == "wave-group" {
									borderColor = u.waveGroupColor(u.disc.entries[i].Group)
								}
								if u.disc.entries[i].Selected {
									borderColor = u.waveSourceColor()
								}
								return widget.Border{Color: borderColor, Width: 1, CornerRadius: 8}.Layout(gtx, inset(8, func(gtx layout.Context) layout.Dimensions {
									l := material.Label(u.textTheme(u.disc.entries[i].Label), 13, u.disc.entries[i].Label)
									if u.disc.entries[i].Action == "wave-option" {
										if u.disc.entries[i].Selected {
											l.Color = u.waveSourceColor()
										}
									}
									l.MaxLines = 2
									gtx.Constraints.Min.Y = gtx.Dp(30)
									gtx.Constraints.Max.Y = gtx.Dp(30)
									if u.disc.entries[i].Action == "wave-option" {
										col := muted
										if u.disc.entries[i].Selected {
											col = u.waveSourceColor()
										}
										paint.FillShape(gtx.Ops, col, clip.Ellipse(image.Rect(0, gtx.Dp(9), gtx.Dp(12), gtx.Dp(21))).Op(gtx.Ops))
										paint.FillShape(gtx.Ops, bg, clip.Ellipse(image.Rect(gtx.Dp(1), gtx.Dp(10), gtx.Dp(11), gtx.Dp(20))).Op(gtx.Ops))
										if u.disc.entries[i].Selected {
											paint.FillShape(gtx.Ops, col, clip.Ellipse(image.Rect(gtx.Dp(3), gtx.Dp(12), gtx.Dp(9), gtx.Dp(18))).Op(gtx.Ops))
										}
										return layout.Inset{Left: 20}.Layout(gtx, l.Layout)
									}
									return l.Layout(gtx)
								}))
							})
							if u.disc.entries[i].Action == "wave-group" {
								above := (i - start) / cols * gtx.Dp(52)
								return u.waveGroupDropdown(gtx, d, u.disc.entries[i].Group, above, availableHeight-above-gtx.Dp(52))
							}
							return d
						})
					})
				}
				return layout.Flex{}.Layout(gtx, cells[:cols]...)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if pages <= 1 {
			return layout.Dimensions{}
		}
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.fit.menuPage == 0 {
				return layout.Dimensions{}
			}
			return u.fitButton(gtx, &u.fit.menuPrevious, "<", false)
		}), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, fmt.Sprintf("%d / %d", u.fit.menuPage+1, pages), 12, muted)
			})
		}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.fit.menuPage+1 == pages {
				return layout.Dimensions{}
			}
			return u.fitButton(gtx, &u.fit.menuNext, ">", false)
		}))
	}))
}
func (u *UI) menuLayout(gtx layout.Context) layout.Dimensions {
	if u.disc.menu == "wave-options" || u.disc.menu == "wave-choice" || u.disc.menu == "musify-chart-settings" {
		return u.panelLayout(gtx, u.waveSettingsPanel)
	}
	return u.panelLayout(gtx, u.menuGrid)
}
func (u *UI) panelLayout(gtx layout.Context, body layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(inset(8, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return u.brand(gtx) }), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.fitButton(gtx, &u.disc.close, "Закрыть", false)
			}))
		})),
		layout.Flexed(1, inset(8, body)),
		layout.Rigid(inset(4, func(gtx layout.Context) layout.Dimensions {
			l := material.Label(u.textTheme(u.status), 11, u.status)
			l.MaxLines = 1
			l.Color = muted
			return l.Layout(gtx)
		})),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if gtx.Constraints.Max.X < gtx.Dp(900) {
				return u.compactPlayer(gtx)
			}
			return u.playerBar(gtx)
		}))
}
func (u *UI) compactPlayer(gtx layout.Context) layout.Dimensions {
	if u.current.ID == "" {
		return layout.Dimensions{}
	}
	return inset(6, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					l := material.Label(u.textTheme(u.current.Title), 12, u.current.Title)
					l.MaxLines = 1
					return l.Layout(gtx)
				}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.playbackClock(gtx, u.player.Snapshot().Position, float64(u.current.DurationMS)/1000, 10)
				}))
			}),
			layout.Rigid(u.phoneTransport),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, u.shuffleButton), gap(4), layout.Flexed(1, u.qualityButton), gap(4), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.trackActionsButton(gtx)
				}))
			}))
	})(gtx)
}
