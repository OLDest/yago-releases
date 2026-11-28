package httpheaders

import (
	"net/http"
	"testing"
)

func TestMediaHasNoAccountIdentity(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://cdn.example/audio", nil)
	Media(r)
	if r.Header.Get("User-Agent") == "" || r.Header.Get("Accept-Encoding") != "identity" {
		t.Fatal("missing media profile")
	}
	for _, key := range []string{"Authorization", "Cookie", "X-Auth-Token", "X-Yandex-Music-Client", "Range"} {
		if r.Header.Get(key) != "" {
			t.Fatal("unexpected media header", key)
		}
	}
}
