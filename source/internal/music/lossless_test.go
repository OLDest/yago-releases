package music

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestLosslessRequestsNativeFLACWithoutMP3Fallback(t *testing.T) {
	for _, codec := range []string{"flac", "mp3"} {
		a := discoveryServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/get-file-info" || r.URL.Query().Get("quality") != "lossless" || r.URL.Query().Get("codecs") != "flac" || r.URL.Query().Get("transports") != "raw" || r.Header.Get("X-Yandex-Music-Client") != "YandexMusicDesktopAppWindows/5.95.0" {
				t.Error("incorrect lossless request")
			}
			fmt.Fprintf(w, `{"result":{"downloadInfo":{"codec":%q,"url":"https://cdn.yandex.net/audio.flac"}}}`, codec)
		})
		info, e := a.Stream(context.Background(), Track{ID: "42"}, Lossless)
		if codec == "flac" {
			if e != nil || info.Codec != "flac" || info.Bitrate != 0 {
				t.Fatal(info, e)
			}
		} else if !errors.Is(e, ErrLosslessUnavailable) {
			t.Fatal("silent lossy fallback", e)
		}
	}
}
