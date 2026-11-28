//go:build windows || android

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
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
	selectClick, accountsClick widget.Clickable
	yandex, zvuk               *music.API
	musify                     *music.Musify
	yandexAccount, zvukAccount music.Account
	authID                     uint64
	restoring                  int
}

func (u *UI) historyOwner(id music.ID) music.ID {
	if music.IsMusify(id) {
		return "musify-guest"
	}
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
	case "musify":
		return []music.ID{"musify-guest"}
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
	result := store.Preferences{Owner: owner, Artists: map[music.ID]string{}, Tracks: map[music.ID]bool{}, TrackNames: map[music.ID]string{}}
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
		for k, name := range p.TrackNames {
			if p.Tracks[k] {
				result.TrackNames[k] = name
			}
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
		part := store.Preferences{Owner: account.UID, Artists: map[music.ID]string{}, Tracks: map[music.ID]bool{}, TrackNames: map[music.ID]string{}}
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
		for k, name := range p.TrackNames {
			if p.Tracks[k] && music.IsZvuk(k) == z {
				part.TrackNames[k] = name
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
	if u.services.mode == "musify" {
		return "Поиск и чарты Musify доступны в гостевом режиме.\nВоспроизведение зависит от доступности сервиса."
	}
	if !u.services.enabled || u.services.mode == "yandex" {
		return "Войдите в Яндекс, чтобы слушать свою волну,\nнаходить треки и открывать плейлисты."
	}
	return "Поиск, любимое и плейлисты — в выбранных сервисах.\nАккаунты подключаются отдельно через меню сверху."
}
func (u *UI) subscriptionDescription() string {
	if u.services.mode == "musify" {
		return "Доступность треков и качество определяет Musify."
	}
	if !u.services.enabled || u.services.mode == "yandex" {
		return "Полные треки доступны с подпиской Яндекс Плюс."
	}
	return "Доступ к полным трекам и HQ зависит от аккаунта и подписки сервиса."
}

func musifySupported() bool { return runtime.GOOS == "windows" || runtime.GOOS == "android" }

func credentialKey(device, provider string) string {
	if provider == "musify" {
		return device + ":musify"
	}
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
	case "musify":
		return "Musify"
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
	if b, e := os.ReadFile(filepath.Join(stateDir, "service-mode")); e == nil && (string(b) == "all" || string(b) == "zvuk" || string(b) == "musify" && musifySupported()) {
		u.services.mode = string(b)
	}
}
func (u *UI) loadServiceCredentials() bool {
	u.services.authID++
	id := u.services.authID
	u.services.restoring = 2
	// Load Musify independently of the selected profile.
	if musifySupported() && !demo {
		if token, err := store.Load(credentialKey(u.device, "musify")); err == nil && token.Service == "musify" {
			if provider, err := newGuestMusify(stateDir); err == nil {
				provider.Client.SetSession(token.Access)
				u.services.musify = provider
			} else {
				log.Print("musify restore provider_error")
			}
		}
	}
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
	case "musify":
		u.api = nil
		u.account = music.Account{}
		if u.services.musify == nil {
			var err error
			u.services.musify, err = newGuestMusify(stateDir)
			if err != nil {
				u.status = "Не удалось открыть Musify: " + err.Error()
				return
			}
		}
		u.api = &music.API{Backend: u.services.musify}
		u.account, _ = u.services.musify.Account(u.ctx)
	case "zvuk":
		u.api = u.services.zvuk
		u.account = u.services.zvukAccount
	case "all":
		r := &music.Services{Yandex: u.services.yandex, Zvuk: u.services.zvuk, YandexAccount: u.services.yandexAccount, ZvukAccount: u.services.zvukAccount}
		if provider := u.services.musify; provider != nil && provider.Client.SignedIn() {
			r.Musify = &music.API{Backend: provider}
		}
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
		if u.services.mode == "all" {
			u.status = "Подключите хотя бы один сервис через меню «Аккаунты»"
		} else {
			u.status = "Подключите " + serviceName(u.services.mode) + " через меню «Аккаунты»"
		}
	} else if u.services.mode == "musify" {
		u.status = "Выберите поиск или чарты"
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
	if u.services.mode == "all" || u.services.mode == "musify" {
		u.accountsMenu()
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
	u.accountsMenu()
	return true
}
func (u *UI) servicesMenu() {
	entries := []menuEntry{{Label: "Общая волна", Action: "service-all"}, {Label: "Яндекс", Action: "service-yandex"}, {Label: "Звук", Action: "service-zvuk"}}
	if musifySupported() {
		entries = append(entries, menuEntry{Label: "Musify", Action: "service-musify"})
	}
	for i := range entries {
		entries[i].Selected = entries[i].Action == "service-"+u.services.mode
		entries[i].Action = "pick-wave-" + strings.TrimPrefix(entries[i].Action, "service-")
	}
	u.showMenu("services", entries)
}
func (u *UI) accountsMenu() {
	var entries []menuEntry
	if musifySupported() {
		if u.services.musify != nil && u.services.musify.Client.SignedIn() {
			entries = append(entries, menuEntry{Label: "Отключить Musify", Action: "service-musify-reset"})
		} else {
			entries = append(entries, menuEntry{Label: "Подключить Musify", Action: "service-musify-login"})
		}
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

	u.showMenu("accounts", entries)
}
func (u *UI) serviceButton(gtx layout.Context) layout.Dimensions {
	if !u.services.enabled {
		return layout.Dimensions{}
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.serviceDropdown(gtx, &u.services.selectClick, "Волна", "services", u.servicesMenu)
		}), gap(6),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.serviceDropdown(gtx, &u.services.accountsClick, "Аккаунты", "accounts", u.accountsMenu)
		}))
}
func (u *UI) serviceDropdown(gtx layout.Context, button *widget.Clickable, label, menu string, open func()) layout.Dimensions {
	if button.Clicked(gtx) {
		if u.disc.menu == menu {
			u.closeMenu()
		} else {
			open()
		}
	}
	d := u.fitButton(gtx, button, label+"  ", false)
	var arrow clip.Path
	arrow.Begin(gtx.Ops)
	x, y := float32(d.Size.X-gtx.Dp(12)), float32(d.Size.Y/2-gtx.Dp(2))
	arrow.MoveTo(f32.Pt(x, y))
	arrow.LineTo(f32.Pt(x+float32(gtx.Dp(6)), y))
	arrow.LineTo(f32.Pt(x+float32(gtx.Dp(3)), y+float32(gtx.Dp(4))))
	arrow.Close()
	paint.FillShape(gtx.Ops, white, clip.Outline{Path: arrow.End()}.Op())
	if u.disc.menu == menu {
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
	if mode, ok := strings.CutPrefix(action, "pick-wave-"); ok {
		if mode == u.services.mode && u.waveAPI() != nil {
			u.closeMenu()
			u.chooseWave()
			return true
		}
		if !u.serviceAction("service-" + mode) {
			return false
		}
		if !u.authBusy && !u.musifyLogin.open && u.waveAPI() != nil {
			u.chooseWave()
		}
		return true
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
		if mode == "musify-playlists" && u.services.musify != nil && u.services.musify.Client.SignedIn() {
			u.closeMenu()
			u.musifyLists.open = true
			u.readMusifyLists()
			return true
		}
		if mode == "musify-login" || mode == "musify-reset" {
			if !musifySupported() {
				return false
			}
			if u.services.musify == nil {
				provider, err := newGuestMusify(stateDir)
				if err != nil {
					u.closeMenu()
					u.status = "Не удалось открыть Musify: " + err.Error()
					return true
				}
				u.services.musify = provider
			}
			u.closeMenu()
			if mode == "musify-reset" {
				if err := store.Delete(credentialKey(u.device, "musify")); err != nil {
					u.status = "Не удалось удалить сохранённую сессию Musify"
					return true
				}
				u.services.musify.Client.SetSession("")
				u.services.musify.ClearHistoryCache()
				if u.services.mode == "all" {
					u.applyServiceMode(false)
				}
				u.status = "Musify отключён в YaGo"
			} else {
				u.musifyLogin.open = true
				u.musifyLogin.login.SingleLine = true
				u.musifyLogin.password.SingleLine = true
				u.musifyLogin.password.Mask = '•'
				u.status = ""
			}
			return true
		}
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
		if mode != "all" && mode != "yandex" && mode != "zvuk" && !(mode == "musify" && musifySupported()) {
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
		switch mode {
		case "yandex":
			if u.services.yandex == nil && u.services.restoring == 0 {
				return u.serviceAction("login-yandex")
			}
		case "zvuk":
			if u.services.zvuk == nil && u.services.restoring == 0 {
				return u.serviceAction("login-zvuk")
			}
		case "musify":
			if u.services.musify != nil && !u.services.musify.Client.SignedIn() {
				return u.serviceAction("service-musify-login")
			}
		}
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
	if u.services.mode == "musify" || music.IsMusify(u.current.ID) {
		return "Качество Musify"
	}
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
	if u.services.mode == "musify" {
		return u.api
	}
	if !u.services.enabled || u.waveMixed() {
		return u.api
	}
	if u.waveIsZvuk() {
		return u.services.zvuk
	}
	return u.services.yandex
}
func (u *UI) waveLabel() string {
	if u.services.mode == "musify" {
		return "Чарты"
	}
	if u.services.enabled && u.services.mode == "all" {
		return "Общая волна"
	}
	if u.waveIsZvuk() {
		return "Мой поток"
	}
	return "Моя волна"
}
func (u *UI) chooseWave() {
	if u.services.mode == "musify" {
		u.openMusifyCharts()
		return
	}
	u.openWave()
}

func (u *UI) restoreWaveSource(id music.ID) {
	if music.IsZvuk(id) {
		u.services.waveSource = "zvuk"
	} else {
		u.services.waveSource = "yandex"
	}
}

func (u *UI) playlistLibraryAPI() *music.API {
	if !u.services.enabled {
		return u.api
	}
	r := &music.Services{Yandex: u.services.yandex, Zvuk: u.services.zvuk, YandexAccount: u.services.yandexAccount, ZvukAccount: u.services.zvukAccount}
	if provider := u.services.musify; provider != nil && provider.Client.SignedIn() {
		r.Musify = &music.API{Backend: provider}
	}
	if r.Yandex == nil && r.Zvuk == nil && r.Musify == nil {
		return nil
	}
	return &music.API{Backend: r}
}

func (u *UI) trackPlaybackAPI(id music.ID) *music.API {
	if u.services.enabled {
		switch {
		case music.IsMusify(id) && u.services.musify != nil:
			return &music.API{Backend: u.services.musify}
		case music.IsZvuk(id) && u.services.zvuk != nil:
			return u.services.zvuk
		case !music.IsMusify(id) && !music.IsZvuk(id) && u.services.yandex != nil:
			return u.services.yandex
		}
	}
	return u.api
}
