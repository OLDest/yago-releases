package zvuk

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"yago/internal/mobile"
	"yago/internal/music"
)

func BrowserLogin(parent context.Context) (LoginResult, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	id, e := mobile.Call("zvukAuthStart", loginURL)
	if e != nil || id == "" {
		return LoginResult{}, errors.New("Не удалось открыть вход в Звук")
	}
	defer mobile.Call("zvukAuthCancel", id)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return LoginResult{}, errors.New("Вход в Звук отменён или время истекло")
		case <-tick.C:
			raw, e := mobile.Call("zvukAuthPoll", id)
			if e != nil {
				return LoginResult{}, e
			}
			if raw == "" {
				continue
			}
			if len(raw) > 64<<10 {
				return LoginResult{}, errors.New("Слишком большой ответ входа")
			}
			var status struct{ Error string }
			if json.Unmarshal([]byte(raw), &status) != nil || status.Error != "" {
				return LoginResult{}, errors.New("Вход в Звук отменён")
			}
			p, e := ParseProfile([]byte(raw))
			if e != nil || p.Token == "" || len(p.Token) > 8192 {
				return LoginResult{}, errors.New("Звук не подтвердил вход")
			}
			name := p.Name
			if name == "" {
				name = "Звук"
			}
			return LoginResult{Token: p.Token, Account: music.Account{UID: ns(p.ID), DisplayName: name, Login: "zvuk"}}, nil
		}
	}
}
