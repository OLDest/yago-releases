package main

import (
	"gioui.org/app"
	gioevent "gioui.org/io/event"
	"log"
	"strings"
	"yago/internal/mobile"
	"yago/internal/winkeys"
)

func (u *UI) initPlatform() {
	emit := func(command string) {
		select {
		case mobile.Commands <- command:
		case <-u.ctx.Done():
		}
	}
	go func() {
		s := <-winkeys.Start(u.ctx, emit)
		if len(s.Unavailable) > 0 {
			log.Print("Hotkeys unavailable: " + strings.Join(s.Unavailable, ", "))
			u.emit(event{kind: "message", message: "Горячие клавиши заняты: " + strings.Join(s.Unavailable, ", ")})
		}
	}()
}
func (u *UI) platformEvent(e gioevent.Event) {
	if v, ok := e.(app.Win32ViewEvent); ok && v.HWND != 0 {
		go u.w.Run(func() {
			if !winkeys.Attach(v.HWND, func(command string) {
				select {
				case mobile.Commands <- command:
				default:
				}
			}) {
				log.Print("Windows app commands unavailable")
			}
		})
	}
}
