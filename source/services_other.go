//go:build !windows && !android

package main

import (
	"gioui.org/layout"
	"yago/internal/music"
	"yago/internal/store"
)

type servicesUI struct{}

func (u *UI) historyOwner(music.ID) music.ID { return u.account.UID }
func (u *UI) historyOwners() []music.ID      { return []music.ID{u.account.UID} }

func (u *UI) profileControls(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if u.api == nil {
			return u.phoneButton(gtx, &u.login, "Войти", true)
		}
		return u.phoneButton(gtx, &u.logout, "Выйти", false)
	})
}

func (u *UI) initServices() {}

func (u *UI) hasWaveService() bool { return true }

func (u *UI) loadServicePreferences(owner music.ID) (store.Preferences, error) {
	return store.LoadPreferences(stateDir, owner)
}
func (u *UI) saveServicePreferences(p store.Preferences) error {
	return store.SavePreferences(stateDir, p)
}
func (u *UI) homeDescription() string {
	return "Войдите в Яндекс, чтобы слушать свою волну,\nнаходить треки и открывать плейлисты."
}
func (u *UI) subscriptionDescription() string {
	return "Полные треки доступны с подпиской Яндекс Плюс."
}

func (u *UI) loadServiceCredentials() bool                   { return false }
func (u *UI) serviceEvent(event) bool                        { return false }
func (u *UI) chooseLogin()                                   { u.startLogin() }
func (u *UI) manageServices() bool                           { return false }
func (u *UI) serviceButton(layout.Context) layout.Dimensions { return layout.Dimensions{} }
func (u *UI) serviceAction(string) bool                      { return false }
func (u *UI) sourceSuffix(_ string) string                   { return "" }
func (u *UI) basicQualityLabel() string                      { return "Basic · MP3 192" }

func (u *UI) waveAPI() *music.API { return u.api }
func (u *UI) waveIsZvuk() bool    { return false }
func (u *UI) waveLabel() string   { return "Моя волна" }
func (u *UI) chooseWave()         { u.openWave() }

func (u *UI) restoreWaveSource(music.ID) {}

func (u *UI) waveMixed() bool { return false }
