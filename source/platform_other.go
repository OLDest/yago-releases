//go:build !windows

package main

import gioevent "gioui.org/io/event"

func (u *UI) initPlatform()                  {}
func (u *UI) platformEvent(e gioevent.Event) {}
