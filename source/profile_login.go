package main

import "context"

// Authentication is independent of the library view and the playing queue.
// Cancelling library navigation here turns a visible Wave into an ordinary list.
func (u *UI) prepareProfileLogin() (context.Context, context.CancelFunc) {
	if u.loginCancel != nil {
		u.loginCancel()
	}
	ctx, cancel := context.WithCancel(u.ctx)
	u.loginCancel = cancel
	u.authBusy = true
	u.code = ""
	return ctx, cancel
}
