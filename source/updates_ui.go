package main

import (
	"context"
	"fmt"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"yago/internal/mobile"
	"yago/internal/updates"
)

type updateUI struct {
	show, busy                          bool
	menu, check, install, cancel, close widget.Clickable
	auto                                widget.Bool
	list                                widget.List
	message, code, path                 string
	client                              *updates.Client
	available                           *updates.Available
	events                              chan func()
	stop                                context.CancelFunc
	serial                              uint64
	next                                time.Time
}

func (u *UI) initUpdates() {
	s := &u.up
	s.events = make(chan func(), 16)
	s.list.Axis = layout.Vertical
	s.auto.Value = true
	s.next = time.Now().Add(30 * time.Minute)
	if b, e := os.ReadFile(filepath.Join(stateDir, "updates-disabled")); e == nil && len(b) > 0 {
		s.auto.Value = false
	}
	s.client = updates.New("")
	s.message = "Обновления из открытого репозитория. Вход не нужен."
	go func() { u.updatePost(func() { u.checkUpdate() }) }()
}
func (u *UI) updatePost(f func()) {
	select {
	case u.up.events <- f:
		u.w.Invalidate()
	case <-u.ctx.Done():
	}
}
func (u *UI) updateJob(message string) (context.Context, uint64) {
	s := &u.up
	s.serial++
	ctx, cancel := context.WithCancel(u.ctx)
	s.stop = cancel
	s.busy = true
	s.message = message
	return ctx, s.serial
}

func (u *UI) finishUpdate(id uint64, f func()) {
	if u.up.serial != id {
		return
	}
	if u.up.stop != nil {
		u.up.stop()
	}
	u.up.stop = nil
	u.up.busy = false
	f()
}
func (u *UI) updateResult(id uint64, f func()) { u.updatePost(func() { u.finishUpdate(id, f) }) }
func updatePlatform() string {
	if runtime.GOOS == "android" {
		return "android_universal"
	}
	return runtime.GOOS + "_" + runtime.GOARCH
}
func (u *UI) checkUpdate() {
	s := &u.up
	if s.busy {
		return
	}
	if s.client == nil {
		s.message = "Источник обновлений недоступен."
		return
	}
	ctx, id := u.updateJob("Проверяем обновления…")
	c := s.client
	s.next = time.Now().Add(30 * time.Minute)
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		a, e := c.Check(ctx, updates.Version, updatePlatform())
		u.updateResult(id, func() {
			if e != nil {
				s.message = e.Error()
				return
			}
			s.available = a
			s.path = ""
			if a == nil {
				s.message = "Установлена последняя версия."
				return
			}
			s.message = "Доступна версия " + a.Manifest.Version
			canDownload := runtime.GOOS != "android"
			if !canDownload {
				v, e := mobile.Call("updateUnmetered", "")
				canDownload = e == nil && v == "true"
			}
			if s.auto.Value && canDownload {
				u.downloadUpdate()
			}
		})
	}()
}
func (u *UI) downloadUpdate() {
	s := &u.up
	if s.busy || s.available == nil || s.client == nil {
		return
	}
	ctx, id := u.updateJob("Загружаем обновление…")
	c, a := s.client, s.available
	go func() {
		last := time.Now()
		path, e := c.Download(ctx, a, filepath.Join(stateDir, "updates"), func(n, total int64) {
			if time.Since(last) < 300*time.Millisecond {
				return
			}
			last = time.Now()
			message := fmt.Sprintf("Загрузка: %d%%", n*100/total)
			u.updatePost(func() {
				if s.serial == id {
					s.message = message
				}
			})
		})
		u.updateResult(id, func() {
			if e != nil {
				s.message = e.Error()
				return
			}
			s.path = path
			s.message = "Обновление проверено. Можно установить."
		})
	}()
}
func (u *UI) updateInputs(gtx layout.Context) {
	s := &u.up
	for s.menu.Clicked(gtx) {
		s.show = !s.show
	}
	for s.close.Clicked(gtx) {
		s.show = false
	}
	for s.check.Clicked(gtx) {
		u.checkUpdate()
	}
	for s.cancel.Clicked(gtx) {
		if s.stop != nil {
			s.stop()
		}
		s.serial++
		s.busy = false
		s.stop = nil
		s.code = ""
		s.message = "Операция отменена."
	}
	for s.install.Clicked(gtx) {
		if s.path == "" {
			u.downloadUpdate()
			continue
		}
		if e := updates.Install(s.path, stateDir); e != nil {
			s.message = e.Error()
		} else if runtime.GOOS == "windows" {
			u.w.Perform(system.ActionClose)
		} else {
			s.message = "Подтвердите установку Android. После разрешения установки нажмите «Установить» ещё раз."
		}
	}
	if s.auto.Update(gtx) {
		p := filepath.Join(stateDir, "updates-disabled")
		if s.auto.Value {
			_ = os.Remove(p)
		} else {
			_ = os.WriteFile(p, []byte("1"), 0600)
		}
	}
}
func (u *UI) tickUpdates() {
	if !u.up.busy && u.up.client != nil && time.Now().After(u.up.next) {
		u.checkUpdate()
	}
}
func (u *UI) updateButton(gtx layout.Context) layout.Dimensions {
	label := "Обновл."
	if u.up.available != nil {
		label = "Новая!"
	}
	return u.phoneButton(gtx, &u.up.menu, label, u.up.available != nil)
}
func (u *UI) updatesContent(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return u.label(gtx, "YaGo "+updates.Version, 22, accent) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.phoneButton(gtx, &u.up.close, "Закрыть", false)
				}))
		}), gap(12), layout.Flexed(1, u.updatesBody))
}
func (u *UI) updatesBody(gtx layout.Context) layout.Dimensions {
	u.ensureCheckboxIcons()
	s := &u.up
	return material.List(u.theme, &s.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.label(gtx, s.message, 14, white) }), gap(12),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.CheckBox(u.theme, &s.auto, "Загружать автоматически (Android: Wi-Fi)").Layout(gtx)
			}), gap(12),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, "Проверка при запуске и каждые 30 минут. Установка — после подтверждения.", 12, muted)
			}), gap(12),
		}

		if s.busy {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &s.cancel, "Отмена", false) }))
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &s.check, "Проверить", false)
			}), gap(8))
			if s.available != nil {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := "Скачать " + s.available.Manifest.Version
					if s.path != "" {
						label = "Установить"
						if runtime.GOOS == "windows" {
							label = "Установить и перезапустить"
						}
					}
					return u.button(gtx, &s.install, label, true)
				}))
			}
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}
