package main

// A new library view cancels the previous fetch, including batched Likes.
// A stale generation must not keep loading data that can no longer be shown.
func (u *UI) cancelLibrary() {
	u.closeExplore()
	u.viewWave = false
	u.off.picking = false
	if u.libraryCancel != nil {
		u.libraryCancel()
		u.libraryCancel = nil
		// Reject responses already queued before cancellation as well.
		u.gen++
	}
}
