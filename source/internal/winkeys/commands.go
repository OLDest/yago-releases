package winkeys

type Binding struct {
	Key, Modifiers uintptr
	Command, Name  string
}

func Bindings() []Binding {
	return []Binding{
		{0xB0, 0, "next", "Media Next"}, {0xB1, 0, "previous", "Media Previous"}, {0xB2, 0, "stop", "Media Stop"}, {0xB3, 0, "toggle", "Media Play/Pause"},
		{0xAF, 0, "volume-up", "Volume Up"}, {0xAE, 0, "volume-down", "Volume Down"}, {0xAD, 0, "mute", "Mute"},
		{0x27, 3, "next", "Ctrl+Alt+Right"}, {0x25, 3, "previous", "Ctrl+Alt+Left"}, {0x20, 3, "toggle", "Ctrl+Alt+Space"},
		{'P', 3, "play", "Ctrl+Alt+P"}, {'O', 3, "pause", "Ctrl+Alt+O"}, {'R', 3, "resume", "Ctrl+Alt+R"}, {'S', 3, "stop", "Ctrl+Alt+S"},
		{0x26, 3, "volume-up", "Ctrl+Alt+Up"}, {0x28, 3, "volume-down", "Ctrl+Alt+Down"}, {'M', 3, "mute", "Ctrl+Alt+M"},
	}
}
func AppCommand(value uintptr) string {
	switch (value >> 16) & 0xFFF {
	case 8:
		return "mute"
	case 9:
		return "volume-down"
	case 10:
		return "volume-up"
	case 11:
		return "next"
	case 12:
		return "previous"
	case 13:
		return "stop"
	case 14:
		return "toggle"
	case 46:
		return "play"
	case 47:
		return "pause"
	}
	return ""
}
