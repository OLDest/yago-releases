package main

func (u *UI) releaseLibraryView() {
	u.setTracks(nil)
	if u.theme != nil {
		u.theme.Shaper = newUIShaper(u.systemFonts)
	}
	u.order = nil
	u.orderPos = 0
	u.qualities = nil
	u.streamBitrate = 0
	u.streamCodec = ""
}
