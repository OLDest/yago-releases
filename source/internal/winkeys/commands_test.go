package winkeys

import "testing"

func TestAppCommands(t *testing.T) {
	for command, want := range map[uintptr]string{8: "mute", 9: "volume-down", 10: "volume-up", 11: "next", 12: "previous", 13: "stop", 14: "toggle", 46: "play", 47: "pause"} {
		for _, device := range []uintptr{0, 0x8000, 0x1000} {
			if got := AppCommand((command | device) << 16); got != want {
				t.Fatalf("%d -> %s", command, got)
			}
		}
	}
	if AppCommand(1<<16) != "" {
		t.Fatal("unrelated command consumed")
	}
}
func TestBindingUniqueness(t *testing.T) {
	seen := map[[2]uintptr]bool{}
	for _, b := range Bindings() {
		k := [2]uintptr{b.Key, b.Modifiers}
		if seen[k] {
			t.Fatal("duplicate binding")
		}
		seen[k] = true
	}
}
