//go:build windows || android

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"yago/internal/music"
	"yago/internal/store"
	"yago/internal/zvuk"
)

type servicesUI struct {
	restoreWarning             string
	enabled                    bool
	mode                       string
	waveSource                 string
	selectClick                widget.Clickable
	yandex, zvuk               *music.API
	yandexAccount, zvukAccount music.Account
	authID                     uint64
	restoring                  int
}

func (u *UI) historyOwner(id music.ID) music.ID {
	if !u.services.enabled {
		return u.account.UID
	}
	if music.IsZvuk(id) {
		return u.services.zvukAccount.UID
	}
	return u.services.yandexAccount.UID
}
func (u *UI) historyOwners() []music.ID {
	if !u.services.enabled {
		return []music.ID{u.account.UID}
	}
	switch u.services.mode {
	case "all":
		return []music.ID{u.services.yandexAccount.UID, u.services.zvukAccount.UID}
	case "zvuk":
		return []music.ID{u.services.zvukAccount.UID}
	default:
		return []music.ID{u.services.yandexAccount.UID}
	}
}

func (u *UI) loadServicePreferences(owner music.ID) (store.Preferences, error) {
	if !u.services.enabled || u.services.mode != "all" {
		return store.LoadPreferences(stateDir, owner)
	}
	result := store.Preferences{Owner: owner, Artists: map[music.ID]string{}, Tracks: map[music.ID]bool{}}
	for _, account := range []music.Account{u.services.yandexAccount, u.services.zvukAccount} {
		if account.UID == "" {
			continue
		}
		p, e := store.LoadPreferences(stateDir, account.UID)
		if e != nil {
			return result, e
		}
		for k, v := range p.Artists {
			result.Artists[k] = v
		}
		for k, v := range p.Tracks {
			result.Tracks[k] = v
		}
		if !music.IsZvuk(account.UID) {
			result.Seeds = p.Seeds
		} else {
			result.ZvukSeeds = p.ZvukSeeds
		}
	}
	return result, result.Validate()
}
func (u *UI) saveServicePreferences(p store.Preferences) error {
	if !u.services.enabled || u.services.mode != "all" {
		return store.SavePreferences(stateDir, p)
	}
	if e := p.Validate(); e != nil {
		return e
	}
	for _, account := range []music.Account{u.services.yandexAccount, u.services.zvukAccount} {
		if account.UID == "" {
			continue
		}
		z := music.IsZvuk(account.UID)
		part := store.Preferences{Owner: account.UID, Artists: map[music.ID]string{}, Tracks: map[music.ID]bool{}}
		for k, v := range p.Artists {
			if music.IsZvuk(k) == z {
				part.Artists[k] = v
			}
		}
		for k, v := range p.Tracks {
			if music.IsZvuk(k) == z {
				part.Tracks[k] = v
			}
		}
		if !z {
			part.Seeds = p.Seeds
		} else {
			part.ZvukSeeds = p.ZvukSeeds
		}
		if e := store.SavePreferences(stateDir, part); e != nil {
			return e
		}
	}
	return nil
}
func (u *UI) homeDescription() string {
	if !u.services.enabled || u.services.mode == "yandex" {
		return "Войдите в Яндекс, чтобы слушать свою волну,\nнаходить треки и открывать плейлисты."
	}
	return "Поиск, любимое и плейлисты — в выбранных сервисах.\nАккаунты подключаются отдельно через меню сверху."
}
func (u *UI) subscriptionDescription() string {
	if !u.services.enabled || u.services.mode == "yandex" {
		return "Полные треки доступны с подпиской Яндекс Плюс."
	}
	return "Доступ к полным трекам и HQ зависит от аккаунта и подписки сервиса."
}

func credentialKey(device, provider string) string {
	if provider == "zvuk" {
		return device + ":zvuk"
	}
	return device
}
func newZvukAPI(t music.Token) *music.API {
	c := zvuk.New(t.Access)
	c.AccountHint = t.Profile
	return &music.API{Backend: c}
}
func serviceName(mode string) string {
	switch mode {
	case "zvuk":
		return "Звук"
	case "all":
		return "Все"
	default:
		return "Яндекс"
	}
}
func (u *UI) initServices() {
	u.services.enabled = true
	if b, e := os.ReadFile(filepath.Join(stateDir, "wave-provider")); e == nil && (string(b) == "zvuk" || string(b) == "yandex") {
		u.services.waveSource = string(b)
	}
	u.services.mode = "yandex"
	if b, e := os.ReadFile(filepath.Join(stateDir, "service-mode")); e == nil && (string(b) == "all" || string(b) == "zvuk") {
		u.services.mode = string(b)
	}
}
func (u *UI) loadServiceCredentials() bool {
	u.services.authID++
	id := u.services.authID
	u.services.restoring = 2
	u.services.restoreWarning = ""
	u.status = "Восстанавливаем сохранённые профили…"
	// Credentials stay in Windows Credential Manager, never in the state directory.
	for _, provider := range []string{"yandex", "zvuk"} {
		go func(provider string) {
			t, e := store.Load(credentialKey(u.device, provider))
			if e != nil {
				u.emit(event{kind: "service-restored-empty", featureID: id})
				return
			}
			t.Service = provider
			a := music.New(t.Access)
			if provider == "zvuk" {
				// Migrate the local build's existing credential without discarding its queue owner.
				if t.Profile == nil {
					if p, err := store.LoadPlayback(stateDir); err == nil && !p.Offline && music.IsZvuk(p.Owner) {
						t.Profile = &music.Account{UID: p.Owner, DisplayName: "Звук", Login: "zvuk"}
					}
				}
				a = newZvukAPI(t)
			}
			ctx, cancel := context.WithTimeout(u.ctx, 30*time.Second)
			defer cancel()
			account, e := a.Account(ctx)
			if e == nil && provider == "zvuk" {
				t.Profile = &account
			}
			if e != nil && provider == "yandex" && t.Refresh != "" {
				if fresh, err := music.Refresh(ctx, t.Refresh); err == nil {
					fresh.Service = provider
					if fresh.Refresh == "" {
						fresh.Refresh = t.Refresh
					}
					t = fresh
					a = music.New(t.Access)
					account, e = a.Account(ctx)
				}
			}
			if b, ok := a.Backend.(*zvuk.Client); ok {
				b.Close()
			}
			u.emit(event{kind: "service-restored", featureID: id, account: account, token: t, err: e})
		}(provider)
	}
	return true
}
func (u *UI) serviceEvent(e event) bool {
	if !u.services.enabled {
		return false
	}
	switch e.kind {
	case "account":
		if e.gen != 0 && e.gen != u.gen {
			return true
		}
		if e.err != nil {
			return false
		}
		u.acceptService(e)
		return true
	case "service-login":
		if e.featureID != u.services.authID {
			return true
		}
		u.authBusy = false
		if e.err != nil {
			u.status = e.err.Error()
			return true
		}
		u.acceptService(e)
		return true
	case "service-restored", "service-restored-empty":
		if e.featureID != u.services.authID {
			return true
		}
		u.services.restoring = max(0, u.services.restoring-1)
		if e.kind == "service-restored" && e.err != nil && e.token.Service == "zvuk" && e.token.Access != "" {
			c := zvuk.New(e.token.Access)
			c.AccountHint = e.token.Profile
			e.account = c.CachedIdentity()
			c.Close()
			u.registerService(e)
			u.services.restoreWarning = "Профиль Звука сохранён; не удалось проверить доступ. " + e.err.Error()
		}
		if e.kind == "service-restored" && e.err == nil {
			u.registerService(e)
			_ = store.Save(credentialKey(u.device, e.token.Service), e.token)
		}
		if u.services.restoring == 0 {
			u.applyServiceMode(false)
			if u.services.restoreWarning != "" {
				u.status = u.services.restoreWarning
			} else if e.err != nil {
				u.status = e.err.Error()
			}
		}
		return true
	}
	return false
}
func (u *UI) registerService(e event) {
	if e.token.Service == "zvuk" {
		if u.services.zvuk != nil {
			if c, ok := u.services.zvuk.Backend.(*zvuk.Client); ok {
				c.Close()
			}
		}
		u.services.zvuk = newZvukAPI(e.token)
		u.services.zvukAccount = e.account
	} else {
		u.services.yandex = music.New(e.token.Access)
		u.services.yandexAccount = e.account
	}
}
func (u *UI) acceptService(e event) {
	if e.token.Service == "zvuk" {
		u.services.restoreWarning = ""
		e.token.Profile = &e.account
	}
	u.registerService(e)
	provider := e.token.Service
	if provider != "zvuk" {
		provider = "yandex"
	}
	if u.services.mode != "all" {
		u.services.mode = provider
	}
	u.authBusy = false
	u.code = ""

	demo = false
	_ = os.WriteFile(filepath.Join(stateDir, "service-mode"), []byte(u.services.mode), 0600)
	u.applyServiceMode(true)
	u.restorePlayback()
	u.status = "Подключён " + serviceName(provider)
	if e := store.Save(credentialKey(u.device, provider), e.token); e != nil {
		u.status = "Вход выполнен только до закрытия: системное хранилище недоступно"
	}
}
func (u *UI) applyServiceMode(reset bool) {
	if reset {
		u.savePlayback(true)
		demo = false
		u.cancelLibrary()
		u.cancelWaveRefresh()
		u.cancelWaveContinuation()
		u.cancelQualityChange()
		u.gen++
		u.streamVersion++
		if u.player != nil {
			u.player.Stop()
		}
		if u.off.manager != nil {
			u.off.manager.Cancel()
		}
		u.current = music.Track{}
		u.queue = nil
		u.index = -1
		u.isWave = false
		u.viewWave = false
		u.queueOffline = false
		u.waveTracks = nil
		u.waveContexts = nil
		u.waveViewContexts = nil
		u.waveViewSession = ""
		u.waveViewBatch = ""
		u.waveViewRefreshed = false
		u.session = ""
		u.batch = ""
		u.order = nil
		u.serviceActive = false
		u.advancePending = false
		u.qualityMenuOpen = false
		u.busy = false

		u.releaseLibraryView()
		u.setTracks(nil)
	}
	switch u.services.mode {
	case "zvuk":
		u.api = u.services.zvuk
		u.account = u.services.zvukAccount
	case "all":
		r := &music.Services{Yandex: u.services.yandex, Zvuk: u.services.zvuk, YandexAccount: u.services.yandexAccount, ZvukAccount: u.services.zvukAccount}
		u.account, _ = r.Account(u.ctx)
		if u.account.UID != "" {
			u.api = &music.API{Backend: r}
		} else {
			u.api = nil
		}
	default:
		u.api = u.services.yandex
		u.account = u.services.yandexAccount
	}
	u.initPreferences()
	u.closeMenu()
	if !u.queueOffline {
		u.page = "home"
		u.title = serviceName(u.services.mode) + " · музыка"
	}
	if u.api == nil {
		u.status = "Подключите " + serviceName(u.services.mode) + " через меню сервиса"
	} else {
		u.status = "Выберите поиск, любимое или плейлист"
	}
	if !reset {
		u.restorePlayback()
	}
}
func (u *UI) chooseLogin() {
	if !u.services.enabled {
		u.startLogin()
		return
	}
	if u.services.mode == "all" {
		u.servicesMenu()
	} else if u.services.mode == "zvuk" {
		u.serviceAction("login-zvuk")
	} else {
		u.serviceAction("login-yandex")
	}
}
func (u *UI) manageServices() bool {
	if !u.services.enabled {
		return false
	}
	u.servicesMenu()
	return true
}
func (u *UI) servicesMenu() {
	entries := []menuEntry{{Label: "Все · общая библиотека", Action: "service-all"}, {Label: "Только Яндекс", Action: "service-yandex"}, {Label: "Только Звук", Action: "service-zvuk"}}
	for i := range entries {
		entries[i].Selected = entries[i].Action == "service-"+u.services.mode
	}
	for _, p := range []struct {
		name string
		api  *music.API
	}{{"yandex", u.services.yandex}, {"zvuk", u.services.zvuk}} {
		if p.api == nil {
			if u.services.restoring > 0 {
				continue
			}
			entries = append(entries, menuEntry{Label: "Подключить " + serviceName(p.name), Action: "login-" + p.name})
		} else {
			entries = append(entries, menuEntry{Label: "Отключить " + serviceName(p.name), Action: "logout-" + p.name})
			if p.name == "zvuk" && u.services.restoreWarning != "" {
				entries = append(entries, menuEntry{Label: "Повторить вход в Звук", Action: "login-zvuk"})
			}
		}
	}
	if u.authBusy {
		entries = append(entries, menuEntry{Label: "Отменить вход", Action: "service-cancel"})
	}
	u.showMenu("services", entries)
}
func (u *UI) serviceButton(gtx layout.Context) layout.Dimensions {
	if !u.services.enabled {
		return layout.Dimensions{}
	}
	if u.services.selectClick.Clicked(gtx) {
		if u.disc.menu == "services" {
			u.closeMenu()
		} else {
			u.servicesMenu()
		}
	}
	d := u.fitButton(gtx, &u.services.selectClick, "Профили  ", false)
	var arrow clip.Path
	arrow.Begin(gtx.Ops)
	x, y := float32(d.Size.X-gtx.Dp(12)), float32(d.Size.Y/2-gtx.Dp(2))
	arrow.MoveTo(f32.Pt(x, y))
	arrow.LineTo(f32.Pt(x+float32(gtx.Dp(6)), y))
	arrow.LineTo(f32.Pt(x+float32(gtx.Dp(3)), y+float32(gtx.Dp(4))))
	arrow.Close()
	paint.FillShape(gtx.Ops, white, clip.Outline{Path: arrow.End()}.Op())
	if u.disc.menu == "services" {
		return u.menuDropdown(gtx, d, true)
	}
	return d
}
func (u *UI) hasWaveService() bool {
	return u.waveAPI() != nil
}
func (u *UI) profileControls(gtx layout.Context) layout.Dimensions {
	if u.services.mode == "all" || u.api == nil || u.account.DisplayName == "" || gtx.Constraints.Max.X < gtx.Dp(900) {
		return layout.Dimensions{}
	}
	return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.label(gtx, u.account.DisplayName, 13, white)
	})
}
func (u *UI) serviceAction(action string) bool {
	if !u.services.enabled {
		return false
	}
	if strings.HasPrefix(action, "wave-source-") {
		u.section = sectionWave
		source := strings.TrimPrefix(action, "wave-source-")
		if source != "yandex" && source != "zvuk" {
			return false
		}
		if (source == "zvuk") == u.waveIsZvuk() {
			u.closeMenu()
			u.openWave()
			return true
		}
		if source == "zvuk" && u.services.zvuk == nil || source == "yandex" && u.services.yandex == nil {
			return true
		}
		u.feedback("skip")
		u.cancelWaveRefresh()
		u.cancelWaveContinuation()
		u.isWave = false
		u.viewWave = false
		u.waveTracks = nil
		u.waveContexts = nil
		u.waveViewContexts = nil
		u.waveViewRefreshed = false
		u.session = ""
		u.batch = ""
		u.services.waveSource = source
		_ = os.WriteFile(filepath.Join(stateDir, "wave-provider"), []byte(source), 0600)
		u.closeMenu()
		u.loadWave(true)
		return true
	}
	if strings.HasPrefix(action, "service-") {
		mode := strings.TrimPrefix(action, "service-")
		if mode == "cancel" {
			if u.loginCancel != nil {
				u.loginCancel()
			}
			u.services.authID++
			u.gen++
			u.authBusy = false
			u.code = ""
			u.closeMenu()
			u.status = "Вход отменён"
			return true
		}
		if mode != "all" && mode != "yandex" && mode != "zvuk" {
			return false
		}
		if u.authBusy {
			if u.loginCancel != nil {
				u.loginCancel()
			}
			u.services.authID++
			u.authBusy = false
			u.code = ""
		}
		u.services.mode = mode
		u.applyServiceMode(true)
		_ = os.WriteFile(filepath.Join(stateDir, "service-mode"), []byte(mode), 0600)
		return true
	}
	if action == "login-yandex" || action == "login-zvuk" {
		u.services.authID++
		u.services.restoring = 0
		u.closeMenu()
		if action == "login-yandex" {
			u.startLogin()
			return true
		}
		ctx, cancel := u.prepareProfileLogin()
		id := u.services.authID
		u.status = "Войдите на сайте Звука в открывшемся окне. Токен сохранится автоматически"
		go func() {
			defer cancel()
			result, e := zvuk.BrowserLogin(ctx)
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			u.emit(event{kind: "service-login", featureID: id, token: music.Token{Service: "zvuk", Access: result.Token}, account: result.Account, err: e})
		}()
		return true
	}
	if action == "logout-yandex" || action == "logout-zvuk" {
		provider := strings.TrimPrefix(action, "logout-")
		if u.loginCancel != nil {
			u.loginCancel()
		}
		u.services.authID++
		u.authBusy = false
		u.code = ""
		if provider == "zvuk" {
			if u.services.zvuk != nil {
				if c, ok := u.services.zvuk.Backend.(*zvuk.Client); ok {
					c.Close()
				}
			}
			u.services.zvuk = nil
			u.services.zvukAccount = music.Account{}
		} else {
			u.services.yandex = nil
			u.services.yandexAccount = music.Account{}
		}
		u.applyServiceMode(true)
		if e := store.Delete(credentialKey(u.device, provider)); e != nil {
			u.status = "Не удалось удалить сохранённый вход"
		} else {
			u.status = "Отключён " + serviceName(provider)
		}
		return true
	}
	return false
}
func (u *UI) sourceSuffix(id string) string {
	if id == "" {
		return ""
	}
	if u.services.mode == "all" || strings.HasPrefix(id, "zvuk-") {
		return " · " + music.Source(music.ID(id))
	}
	return ""
}
func (u *UI) basicQualityLabel() string {
	if u.services.mode == "zvuk" || music.IsZvuk(u.current.ID) {
		return "Basic · MP3 128"
	}
	if u.services.mode == "all" {
		return "Basic · по сервису"
	}
	return "Basic · MP3 192"
}

func (u *UI) waveIsZvuk() bool {
	return u.services.enabled && u.services.mode == "zvuk"
}
func (u *UI) waveMixed() bool { return u.services.enabled && u.services.mode == "all" }
func (u *UI) waveAPI() *music.API {
	if !u.services.enabled || u.waveMixed() {
		return u.api
	}
	if u.waveIsZvuk() {
		return u.services.zvuk
	}
	return u.services.yandex
}
func (u *UI) waveLabel() string {
	if u.services.enabled && u.services.mode == "all" {
		return "Общая волна"
	}
	if u.waveIsZvuk() {
		return "Мой поток"
	}
	return "Моя волна"
}
func (u *UI) chooseWave() { u.openWave() }

func (u *UI) restoreWaveSource(id music.ID) {
	if music.IsZvuk(id) {
		u.services.waveSource = "zvuk"
	} else {
		u.services.waveSource = "yandex"
	}
}
