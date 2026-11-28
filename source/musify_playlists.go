package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"yago/internal/music"
)

type musifyListsUI struct {
	open, busy, deleting                               bool
	lists                                              []music.Playlist
	rows                                               []widget.Clickable
	selected                                           music.ID
	list                                               widget.List
	name                                               widget.Editor
	create, rename, add, remove, delete, close, manage widget.Clickable
}

func (u *UI) readMusifyLists() {
	p := u.services.musify
	u.musifyLists.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(u.ctx, 20*time.Second)
		defer cancel()
		lists, err := p.Playlists(ctx, "musify-guest")
		u.emit(event{kind: "musify-playlists-read", playlists: lists, err: err})
	}()
}

func (u *UI) musifyListsLayout(gtx layout.Context) layout.Dimensions {
	f := &u.musifyLists
	f.name.SingleLine = true
	f.list.Axis = layout.Vertical
	for f.close.Clicked(gtx) {
		if !f.busy {
			f.open = false
			f.deleting = false
			f.name.SetText("")
		}
	}
	for i := range f.rows {
		if f.rows[i].Clicked(gtx) {
			f.selected = f.lists[i].Kind
			f.deleting = false
		}
	}
	parse := func(id music.ID) int64 {
		raw, ok := strings.CutPrefix(string(id), "musify-")
		if !ok {
			return 0
		}
		n, _ := strconv.ParseInt(raw, 10, 64)
		return n
	}
	for _, control := range []struct {
		b  *widget.Clickable
		op string
	}{
		{&f.create, "create"}, {&f.rename, "rename"}, {&f.add, "add"}, {&f.remove, "remove"}, {&f.delete, "delete"},
	} {
		if !control.b.Clicked(gtx) || f.busy {
			continue
		}
		name := strings.TrimSpace(f.name.Text())
		id, song := parse(f.selected), parse(u.current.ID)
		if control.op != "create" && id <= 0 {
			u.status = "Выберите плейлист Musify"
			continue
		}
		if (control.op == "create" || control.op == "rename") && name == "" {
			u.status = "Введите название"
			continue
		}
		if (control.op == "add" || control.op == "remove") && song <= 0 {
			u.status = "Выберите текущий трек Musify"
			continue
		}
		if control.op == "delete" && !f.deleting {
			f.deleting = true
			u.status = "Нажмите «Подтвердить удаление»"
			continue
		}
		f.deleting = false
		f.busy = true
		client := u.services.musify.Client
		go func(op string) {
			ctx, cancel := context.WithTimeout(u.ctx, 20*time.Second)
			defer cancel()
			err := client.EditPlaylist(ctx, op, id, song, name)
			u.emit(event{kind: "musify-playlist-result", err: err})
		}(control.op)
	}
	button := func(b *widget.Clickable, label string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, b, label, false) })
	}
	actions := func(gtx layout.Context, buttons ...layout.FlexChild) layout.Dimensions {
		if gtx.Constraints.Max.X < gtx.Dp(500) {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, buttons...)
		}
		return layout.Flex{}.Layout(gtx, buttons...)
	}
	deleteLabel := "Удалить"
	if f.deleting {
		deleteLabel = "Подтвердить удаление"
	}
	return inset(12, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, "Плейлисты Musify", 20, white)
				}), button(&f.close, "Назад"))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Editor(u.theme, &f.name, "Название плейлиста").Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return actions(gtx, button(&f.create, "Создать"), gap(6), button(&f.rename, "Переименовать"), gap(6), button(&f.delete, deleteLabel))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return actions(gtx, button(&f.add, "Добавить текущий трек"), gap(6), button(&f.remove, "Убрать текущий трек"))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.label(gtx, u.status, 12, muted) }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.List(u.theme, &f.list).Layout(gtx, len(f.lists), func(gtx layout.Context, i int) layout.Dimensions {
					return u.button(gtx, &f.rows[i], f.lists[i].Title, f.lists[i].Kind == f.selected)
				})
			}),
		)
	})(gtx)
}
