package music

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCredentialHostBoundary(t *testing.T) {
	for _, s := range []string{"https://api.music.yandex.net/tracks", "https://oauth.yandex.ru/token"} {
		if !Trusted(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"http://api.music.yandex.net", "https://yandex.net.evil.test", "https://notyandex.ru", "https://yandex.ru@evil.test", "file:///yandex.net"} {
		if Trusted(s) {
			t.Fatal(s)
		}
	}
}
func TestMixedTrackIdentifiers(t *testing.T) {
	var v []Track
	if e := json.Unmarshal([]byte(`[{"id":123,"title":"A","albums":[{"id":"42"}]},{"id":"456"}]`), &v); e != nil {
		t.Fatal(e)
	}
	if v[0].FullID() != "123:42" || v[1].FullID() != "456" {
		t.Fatal(v)
	}
}
func TestSearchDoesNotSendTokenToUntrustedHost(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("credential leak")
		}
		if r.URL.Query().Get("text") != "a & б" {
			t.Error(r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":{"tracks":{"results":[{"id":1,"title":"Track"}]}}}`))
	}))
	defer s.Close()
	a := New("private-token")
	a.Base = s.URL
	v, e := a.Search(context.Background(), "a & б")
	if e != nil || len(v) != 1 || v[0].ID != "1" {
		t.Fatal(v, e)
	}
}
func TestSearchErrors(t *testing.T) {
	for _, code := range []int{401, 403, 429, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
			defer s.Close()
			a := New("secret")
			a.Base = s.URL
			if _, e := a.Search(context.Background(), "test"); e == nil {
				t.Fatal("expected error")
			}
		})
	}
}
