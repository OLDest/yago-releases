package music

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWaveResponseShapes(t *testing.T) {
	payload := `{"radioSessionId":"session-1","batchId":"batch-1","sequence":[{"track":{"id":123,"title":"One"}},{"track":{"id":"456","title":"Two"}}]}`
	for _, tc := range []struct{ name, response, session, route string }{
		{"wrapped_new", `{"result":` + payload + "}", "", "/rotor/session/new"},
		{"direct_new", payload, "", "/rotor/session/new"},
		{"direct_continue", `{"batchId":"batch-1","sequence":[{"track":{"id":"123","title":"One"}},{"track":{"id":456,"title":"Two"}}]}`, "session-1", "/rotor/session/session-1/tracks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != tc.route {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("X-Yandex-Music-Without-Invocation-Info") != "1" {
					t.Error("missing session header")
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if tc.session == "" && body["seeds"] == nil {
					t.Error("new session missing seeds")
				}
				if tc.session != "" && body["queue"] == nil {
					t.Error("continuation missing queue")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			api := New("not-a-real-token")
			api.Base = server.URL
			wave, err := api.Wave(context.Background(), tc.session, []string{"previous"})
			if err != nil {
				t.Fatal(err)
			}
			if wave.Session != "session-1" || wave.Batch != "batch-1" || len(wave.Sequence) != 2 || wave.Sequence[0].Track.ID != "123" {
				t.Fatalf("incorrect wave: %#v", wave)
			}
		})
	}
}

func TestWaveRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{
		"{}",
		`{"result":null}`,
		`{"radioSessionId":"session-1","sequence":[]}`,
		`{"radioSessionId":"session-1","sequence":[{"track":null}]}`,
		`{"sequence":[{"track":{"id":"123"}}]}`,
		`{"error":{"name":"SESSION_EXPIRED"},"sequence":[{"track":{"id":"123"}}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			api := New("")
			api.Base = server.URL
			if _, err := api.Wave(context.Background(), "", nil); err == nil {
				t.Fatal("invalid wave accepted")
			}
		})
	}
}

func TestDirectWaveDoesNotRelaxOtherEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tracks":{"results":[{"id":"123"}]}}`))
	}))
	defer server.Close()
	api := New("")
	api.Base = server.URL
	if _, err := api.Search(context.Background(), "test"); err == nil || !strings.Contains(err.Error(), "пустой") {
		t.Fatalf("non-session envelope validation changed: %v", err)
	}
}

func TestWaveFeedbackAcceptsNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	api := New("")
	api.Base = server.URL
	if err := api.Feedback(context.Background(), "session-1", "batch-1", "trackStarted", Track{ID: "123"}, 0); err != nil {
		t.Fatal(err)
	}
}
