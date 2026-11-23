package music

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamFallbackRequestsMP3(t *testing.T) {
	legacy, modern := false, false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tracks/123/download-info" {
			legacy = true
			w.WriteHeader(404)
			return
		}
		if r.URL.Path != "/get-file-info" {
			t.Error(r.URL.Path)
		}
		modern = true
		q := r.URL.Query()
		if q.Get("trackId") != "123" || q.Get("codecs") != "mp3" || q.Get("transports") != "raw" || len(q.Get("sign")) != 43 {
			t.Error("wrong stream negotiation", q)
		}
		w.Write([]byte(`{"result":{"downloadInfo":{"codec":"mp3","url":"https://cdn.yandex.net/test.mp3"}}}`))
	}))
	defer s.Close()
	a := New("secret")
	a.Base = s.URL
	raw, e := a.StreamURL(context.Background(), Track{ID: "123"})
	if e != nil || raw != "https://cdn.yandex.net/test.mp3" || !legacy || !modern {
		t.Fatal(raw, e, legacy, modern)
	}
}
func TestStreamFallbackRejectsForeignCDN(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/get-file-info" {
			w.Write([]byte(`{"result":{"downloadInfo":{"codec":"mp3","url":"https://evil.test/test.mp3"}}}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer s.Close()
	a := New("secret")
	a.Base = s.URL
	if _, e := a.StreamURL(context.Background(), Track{ID: "123"}); e == nil {
		t.Fatal("foreign CDN allowed")
	}
}
