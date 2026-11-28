package zvuk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"yago/internal/music"
)

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New("test-secret")
	c.Base = srv.URL
	t.Cleanup(c.Close)
	return c
}
func TestProfileRejectsAnonymousAndSupportsBothShapes(t *testing.T) {
	for _, s := range []string{`{"result":{"id":2,"name":"Имя","token":"secret","is_anonymous":false}}`, `{"result":{"profile":{"id":"2","name":"Имя","token":"secret","is_anonymous":false}}}`} {
		p, e := ParseProfile([]byte(s))
		if e != nil || p.ID != "2" || p.Name != "Имя" {
			t.Fatal(p.ID, e)
		}
	}
	for _, s := range []string{`{"result":{"id":1,"token":"anon","is_anonymous":true}}`, `{"result":{"id":2}}`, `{}`, `<html>secret</html>`} {
		if _, e := ParseProfile([]byte(s)); e == nil {
			t.Fatal("accepted unauthenticated profile")
		}
	}
}
func TestStreamAndNamespaceIsolation(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Auth-Token") != "test-secret" || r.Header.Get("Authorization") != "" {
			t.Error("wrong credential header")
		}
		if r.URL.Path != "/api/v1/graphql" {
			t.Error("blocked Tiny endpoint used before GraphQL")
		}
		var in struct{ Variables struct{ IDs []string } }
		_ = json.NewDecoder(r.Body).Decode(&in)
		if len(in.Variables.IDs) != 1 || in.Variables.IDs[0] != "123" {
			t.Error("ID not stripped")
		}
		fmt.Fprint(w, `{"data":{"mediaContents":[{"stream":{"high":"https://cdn.example/high.mp3","mid":"https://cdn.example/mid.mp3"}}]}}`)
	})
	for _, q := range []music.Quality{music.Normal, music.High} {
		s, e := c.Stream(context.Background(), music.Track{ID: "zvuk-123"}, q)
		if e != nil || s.URL == "" {
			t.Fatal(e)
		}
		if q == music.Normal && s.Bitrate != 128 {
			t.Fatal(s.Bitrate)
		}
	}
	if _, e := c.Stream(context.Background(), music.Track{ID: "123"}, music.High); e == nil || calls != 2 {
		t.Fatal("foreign ID reached Zvuk")
	}
}
func TestStreamFallsBackToTinyWhenGraphQLUnavailable(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.WriteHeader(503)
			return
		}
		if r.URL.Query().Get("id") != "9" || r.URL.Query().Get("quality") != "mid" {
			t.Error(r.URL.Path)
		}
		fmt.Fprint(w, `{"result":{"stream":"https://cdn.example/mid.mp3"}}`)
	})
	s, e := c.Stream(context.Background(), music.Track{ID: "zvuk-9"}, music.Normal)
	if e != nil || s.Bitrate != 128 {
		t.Fatal(e)
	}
}
func TestHQUsesAvailableMidWithoutDRM(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"mediaContents":[{"stream":{"mid":"https://cdn.example/mid.mp3","high":null,"flacdrm":"https://cdn.example/drm"}}]}}`)
	})
	s, e := c.Stream(context.Background(), music.Track{ID: "zvuk-9"}, music.High)
	if e != nil || s.Bitrate != 128 || strings.Contains(s.URL, "drm") {
		t.Fatal("quality fallback", e)
	}
}
func TestNoTokenOnRedirectAndSanitizedErrors(t *testing.T) {
	leaks := 0
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaks++ }))
	defer other.Close()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/?secret=test-secret", http.StatusFound)
	})
	_, e := c.Account(context.Background())
	if e == nil || strings.Contains(e.Error(), "test-secret") || leaks != 0 {
		t.Fatal("redirect/secret exposure", e, leaks)
	}
}
func TestAccountUsesAuthenticatedCollectionWhenProfileBlocked(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(418)
			return
		}
		fmt.Fprint(w, `{"data":{"collection":{"playlists":[]}}}`)
	})
	c.AccountHint = &music.Account{UID: "zvuk-verified-account", DisplayName: "Звук"}
	a, e := c.Account(context.Background())
	if e != nil || a.UID != c.AccountHint.UID {
		t.Fatal("profile fallback lost identity", e)
	}
}
func TestAccountRejectsInvalidCollectionAfterBlockedProfile(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(418)
			return
		}
		fmt.Fprint(w, `{"errors":[{"message":"invalid token"}]}`)
	})
	c.AccountHint = &music.Account{UID: "zvuk-old-account"}
	if _, e := c.Account(context.Background()); e == nil {
		t.Fatal("cached identity accepted without server validation")
	}
}
func TestErrorsAndResponseLimits(t *testing.T) {
	for _, mode := range []string{"418", "graphql", "oversize", "empty", "html"} {
		t.Run(mode, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "418":
					w.WriteHeader(418)
					fmt.Fprint(w, "secret")
				case "graphql":
					fmt.Fprint(w, `{"errors":[{"message":"test-secret"}]}`)
				case "oversize":
					fmt.Fprint(w, strings.Repeat(" ", responseLimit+1))
				case "html":
					fmt.Fprint(w, "<html>test-secret</html>")
				default:
					fmt.Fprint(w, `{"data":null}`)
				}
			})
			_, e := c.Tracks(context.Background(), []string{"zvuk-1"})
			if e == nil || strings.Contains(e.Error(), "test-secret") {
				t.Fatal(e)
			}
		})
	}
}
func TestSearchCursorBoundsAndExhaustedGroups(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Query     string
			Variables map[string]any
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		calls++
		if calls > 1 && strings.Contains(in.Query, "artists(limit:") {
			t.Error("exhausted group restarted")
		}
		// Every cursor variable in the declaration must also occur in a field.
		for _, name := range []string{"tc", "ac", "rc", "pc"} {
			if strings.Contains(in.Query, "$"+name+":Cursor") && !strings.Contains(in.Query, "cursor:$"+name) {
				t.Error("unused variable", name)
			}
		}
		page := 0
		if s, ok := in.Variables["tc"].(string); ok {
			fmt.Sscanf(s, "cursor-%d", &page)
		}
		fmt.Fprintf(w, `{"data":{"search":{"tracks":{"page":{"next":%d,"cursor":"cursor-%d"},"items":[{"id":%d,"title":"Песня","duration":12,"artists":[{"id":3,"title":"Автор"}],"release":{"id":4,"title":"Альбом"}}]}}}}`, page+1, page+1, page+1)
	})
	for page := 0; page < 30; page++ {
		r, e := c.SearchPage(context.Background(), "test", "all", page)
		if e != nil || len(r.Tracks) != 1 || r.Tracks[0].DurationMS != 12000 || r.Tracks[0].Artists[0].ID != "zvuk-3" {
			t.Fatal(page, e)
		}
		if len(c.cursors) > 8 {
			t.Fatal("cursor growth", len(c.cursors))
		}
	}
	// A far previous page can be reconstructed without retaining all responses.
	calls = 0
	if r, e := c.SearchPage(context.Background(), "test", "all", 1); e != nil || r.Tracks[0].ID != "zvuk-2" {
		t.Fatal(e, r)
	}
}
func TestMutationsAndLibrarySchema(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Query     string
			Variables map[string]any
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if strings.Contains(in.Query, "DATE_ADDED") || strings.Contains(in.Query, "DESC") {
			t.Error("wrong enum values")
		}
		if strings.Contains(in.Query, "mutation") {
			if in.Variables["id"] != "9" || in.Variables["type"] != "artist" {
				t.Error(in.Variables)
			}
			fmt.Fprint(w, `{"data":{"hidden_collection":{"addItem":true}}}`)
		} else {
			fmt.Fprint(w, `{"data":{"collection":{"tracks":[]}}}`)
		}
	})
	if _, e := c.Likes(context.Background(), "zvuk-user"); e != nil {
		t.Fatal(e)
	}
	if e := c.Dislike(context.Background(), "zvuk-user", "artist", "zvuk-9", false); e != nil {
		t.Fatal(e)
	}
}
