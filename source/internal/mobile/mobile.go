package mobile

import "encoding/json"

// Commands arrive from MediaSession, notification buttons and headset keys.
var Commands = make(chan string, 32)

type Playback struct {
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Duration int64  `json:"duration"`
	Position int64  `json:"position"`
	Playing  bool   `json:"playing"`
	Loading  bool   `json:"loading"`
	Active   bool   `json:"active"`
}

func Publish(p Playback) error {
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	_, e = Call("publish", string(b))
	return e
}
func OpenURL(raw string) error { _, e := Call("openURL", raw); return e }
