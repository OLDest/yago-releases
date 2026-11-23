package main

import (
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func padding(i layout.Inset, w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return i.Layout(gtx, w) }
}
func (u *UI) phoneButton(gtx layout.Context, b *widget.Clickable, label string, primary bool) layout.Dimensions {
	v := material.Button(u.textTheme(label), b, label)
	v.TextSize = 12
	v.CornerRadius = 10
	v.Inset = layout.Inset{Top: 12, Bottom: 12, Left: 8, Right: 8}
	if !primary {
		v.Background = panel
		v.Color = white
	}
	return v.Layout(gtx)
}
func (u *UI) phoneLayout(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(inset(12, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return u.brand(gtx) }),
				layout.Rigid(u.serviceButton), gap(6), layout.Rigid(u.updateButton), layout.Rigid(u.profileControls))
		})),
		layout.Rigid(padding(layout.Inset{Left: 12, Right: 12, Bottom: 6}, u.phoneSearch)),
		layout.Rigid(padding(layout.Inset{Left: 12, Right: 12, Bottom: 6}, u.phoneNavigation)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if u.fit.compact {
				return layout.Dimensions{}
			}
			return u.offlineToolbar(gtx)
		}),
		layout.Rigid(u.discoveryToolbar),
		layout.Rigid(padding(layout.Inset{Left: 12, Right: 12, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
			return u.viewHeading(gtx, 18)
		})),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.swipeContent(gtx, u.pagedContent)
			})
		}),
		layout.Rigid(inset(4, func(gtx layout.Context) layout.Dimensions {
			status := u.status
			if s := u.player.Snapshot(); s.Error != "" {
				status = s.Error
			}
			l := material.Label(u.textTheme(status), 11, status)
			l.Color = muted
			l.MaxLines = 2
			return l.Layout(gtx)
		})),
		layout.Rigid(u.phonePlayer))
}
func (u *UI) phoneSearch(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return widget.Border{Color: muted, CornerRadius: 10, Width: 1}.Layout(gtx, inset(10, func(gtx layout.Context) layout.Dimensions {
				e := material.Editor(u.textTheme(u.editor.Text()), &u.editor, "Трек или исполнитель")
				e.TextSize = 14
				return e.Layout(gtx)
			}))
		}), gap(8),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.phoneButton(gtx, &u.search, "Найти", true) }))
}
func (u *UI) phoneNavigation(gtx layout.Context) layout.Dimensions {
	button := func(gtx layout.Context, b *widget.Clickable, label string, primary bool) layout.Dimensions {
		v := material.Button(u.textTheme(label), b, label)
		v.TextSize = 11
		v.CornerRadius = 10
		v.Inset = layout.Inset{Top: 12, Bottom: 12, Left: 5, Right: 5}
		if b == &u.wave && primary {
			v.Background = u.waveSourceColor()
		}
		if !primary {
			v.Background = panel
			v.Color = white
		}
		return v.Layout(gtx)
	}
	return layout.Flex{}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !u.hasWaveService() {
				return layout.Dimensions{}
			}
			d := button(gtx, &u.wave, u.waveLabel(), u.section == sectionWave)
			if u.disc.menu == "waves" {
				return u.menuDropdown(gtx, d, true)
			}
			return d
		}), gap(6),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return button(gtx, &u.likes, "Любимое", u.section == sectionLikes)
		}), gap(6),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return button(gtx, &u.lists, "Плейлисты", u.section == sectionPlaylists)
		}), gap(6), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return button(gtx, &u.off.open, "Оффлайн", u.section == sectionOffline)
		}))
}
func (u *UI) phonePlayer(gtx layout.Context) layout.Dimensions {
	if u.fit.compact {
		return u.compactPlayer(gtx)
	}
	if u.current.ID == "" {
		return inset(12, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, "Выберите музыку", 12, muted)
			}), gap(6), layout.Rigid(u.qualityButton))
		})(gtx)
	}
	if u.page == "offline" || u.off.picking || u.off.removing != "" {
		return widget.Border{Color: muted, Width: 1}.Layout(gtx, inset(8, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Label(u.textTheme(u.current.Title), 12, u.current.Title)
				l.MaxLines = 1
				return l.Layout(gtx)
			}), gap(6), layout.Rigid(u.phoneTransport))
		}))
	}
	s := u.player.Snapshot()
	return widget.Border{Color: muted, Width: 1}.Layout(gtx, inset(12, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Label(u.textTheme(u.current.Title), 14, u.current.Title)
				l.MaxLines = 1
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				artist := u.current.Artist() + u.sourceSuffix(string(u.current.ID)) + u.streamLabel()
				if s.Loading {
					artist = "Загрузка…"
				}
				l := material.Label(u.textTheme(artist), 12, artist)
				l.MaxLines = 1
				l.Color = muted
				return l.Layout(gtx)
			}), gap(8),
			layout.Rigid(u.phoneTransport), gap(8),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				value := float32(0)
				if u.current.DurationMS > 0 {
					value = float32(s.Position / (float64(u.current.DurationMS) / 1000))
				}
				p := material.ProgressBar(u.theme, max(0, min(1, value)))
				p.Color = accent
				return p.Layout(gtx)
			}), gap(4),
			layout.Rigid(u.phoneTime), gap(4), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx, layout.Flexed(1, u.shuffleButton), gap(6), layout.Flexed(1, u.qualityButton), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.trackActionsButton(gtx)
				}))
			}))
	}))
}
func (u *UI) phoneTransport(gtx layout.Context) layout.Dimensions {
	s := u.player.Snapshot()
	return layout.Flex{}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return u.phoneButton(gtx, &u.previous, "Назад", false)
		}), gap(6),
		layout.Flexed(1.2, func(gtx layout.Context) layout.Dimensions {
			label := "Играть"
			if s.Playing {
				label = "Пауза"
			}
			return u.phoneButton(gtx, &u.pause, label, true)
		}), gap(6),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return u.phoneButton(gtx, &u.next, "Далее", false) }), gap(6),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.phoneButton(gtx, &u.like, "♥", false) }))
}
func (u *UI) phoneTime(gtx layout.Context) layout.Dimensions {
	s := u.player.Snapshot()
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.phoneButton(gtx, &u.rewind, "−15 с", false) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.playbackClock(gtx, s.Position, float64(u.current.DurationMS)/1000, 11)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.phoneButton(gtx, &u.forward, "+15 с", false) }))
}
