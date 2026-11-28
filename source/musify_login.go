package main

import (
	"context"
	"errors"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"strings"
	"time"
	"yago/internal/music"
	"yago/internal/store"
)

type musifyLoginUI struct {
	open, busy      bool
	login, password widget.Editor
	submit, back    widget.Clickable
}

func (u *UI) musifyLoginLayout(gtx layout.Context) layout.Dimensions {
	form := &u.musifyLogin
	for form.back.Clicked(gtx) {
		if !form.busy {
			form.open = false
			form.password.SetText("")
		}
	}
	for form.submit.Clicked(gtx) {
		if form.busy {
			continue
		}
		login, password := strings.TrimSpace(form.login.Text()), form.password.Text()
		if login == "" || password == "" {
			continue
		}
		form.password.SetText("")
		form.busy = true
		u.status = "Вход в Musify…"
		client := u.services.musify.Client
		go func() {
			ctx, cancel := context.WithTimeout(u.ctx, 20*time.Second)
			defer cancel()
			token, err := client.Login(ctx, login, password)
			if err == nil {
				if saveErr := store.Save(credentialKey(u.device, "musify"), music.Token{Service: "musify", Access: token}); saveErr != nil {
					err = errors.New("Не удалось сохранить сессию Musify")
				} else {
					client.SetSession(token)
				}
			}
			u.emit(event{kind: "musify-login-result", err: err})
		}()
	}
	return inset(20, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Editor(u.theme, &form.login, "Логин Musify").Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Editor(u.theme, &form.password, "Пароль").Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &form.submit, "Войти", true) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &form.back, "Назад", false) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return material.Body1(u.theme, u.status).Layout(gtx) }),
		)
	})(gtx)
}
