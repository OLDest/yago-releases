package music

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discoveryServer(t *testing.T, h http.HandlerFunc) *API {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	a := New("")
	a.Base = s.URL
	return a
}
func TestDiscoveryEndpoints(t *testing.T) {
	calls := 0
	a := discoveryServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/search":
			if r.URL.Query().Get("type") != "album" || r.URL.Query().Get("text") != "a & b" || r.URL.Query().Get("page") != "1" {
				t.Error("search query", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"result":{"albums":{"total":100,"perPage":40,"results":[{"id":4,"title":"Album"}]}}}`)
		case "/search/suggest":
			fmt.Fprint(w, `{"result":{"suggestions":["a","b","c","d","e","f"]}}`)
		case "/users/owner/likes/tracks/remove":
			r.ParseForm()
			if r.Method != "POST" || r.Form.Get("track-ids") != "song:album" {
				t.Error("unlike form")
			}
			fmt.Fprint(w, `{"result":{"revision":2}}`)
		case "/users/owner/dislikes/artists/add-multiple":
			r.ParseForm()
			if r.Form.Get("artist-ids") != "artist" {
				t.Error("artist form")
			}
			fmt.Fprint(w, `{"result":"ok"}`)
		case "/users/owner/dislikes/tracks/remove":
			r.ParseForm()
			if r.Form.Get("track-ids") != "song" {
				t.Error("track form")
			}
			fmt.Fprint(w, `{"result":{"revision":3}}`)
		default:
			t.Error("unexpected endpoint", r.URL)
			http.Error(w, "unexpected", 500)
		}
	})
	p, e := a.SearchPage(context.Background(), "a & b", "album", 1)
	if e != nil || !p.More || len(p.Albums) != 1 || p.Albums[0].ID != "4" {
		t.Fatal(p, e)
	}
	s, e := a.Suggest(context.Background(), "a")
	if e != nil || len(s) != 5 {
		t.Fatal(s, e)
	}
	if e = a.Unlike(context.Background(), "owner", Track{ID: "song", Albums: []Album{{ID: "album"}}}); e != nil {
		t.Fatal(e)
	}
	if e = a.Dislike(context.Background(), "owner", "artist", "artist", false); e != nil {
		t.Fatal(e)
	}
	if e = a.Dislike(context.Background(), "owner", "track", "song", true); e != nil {
		t.Fatal(e)
	}
	if calls != 5 {
		t.Fatal(calls)
	}
}
func TestAlbumPagesDoNotLoseTracksAcrossDiscs(t *testing.T) {
	a := discoveryServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"volumes":[[`)
		for i := 0; i < 115; i++ {
			if i > 0 {
				if i == 50 {
					fmt.Fprint(w, "],[")
				} else {
					fmt.Fprint(w, ",")
				}
			}
			fmt.Fprintf(w, `{"id":"%d","title":"Song"}`, i)
		}
		fmt.Fprint(w, `]]}}`)
	})
	for page := 0; page < 3; page++ {
		tracks, more, e := a.AlbumPage(context.Background(), "album", page)
		want := min(40, 115-page*40)
		if e != nil || len(tracks) != want || tracks[0].ID != ID(fmt.Sprint(page*40)) || more != (page < 2) {
			t.Fatal(page, len(tracks), more, e)
		}
	}
}
func TestWaveUsesServerSerializedSeeds(t *testing.T) {
	a := discoveryServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rotor/wave/settings":
			fmt.Fprint(w, `{"result":{"blocks":[{"type":"contexts","items":[{"name":"Бег","id":{"type":"activity","tag":"running"}}]}],"settingRestrictions":{"diversity":{"name":"Треки","possibleValues":[{"name":"Открытия","serializedSeed":"settingDiversity:discover"},{"name":"Unsupported"}]}}}}`)
		case "/rotor/session/new":
			var b struct {
				Seeds, Queue     []string
				IncludeWaveModel bool
			}
			json.NewDecoder(r.Body).Decode(&b)
			if strings.Join(b.Seeds, ",") != "activity:running,settingDiversity:discover" || b.IncludeWaveModel || len(b.Queue) != 1 {
				t.Error("wrong session body", b)
			}
			fmt.Fprint(w, `{"radioSessionId":"s","sequence":[{"track":{"id":1}}]}`)
		}
	})
	o, e := a.WaveOptions(context.Background())
	if e != nil || len(o) != 3 || o[2].Seed != "settingDiversity:discover" {
		t.Fatal(o, e)
	}
	_, e = a.WaveWithSeeds(context.Background(), "", []string{"played"}, []string{o[1].Seed, o[2].Seed})
	if e != nil {
		t.Fatal(e)
	}
}
func TestHistoryBoundedAndPaged(t *testing.T) {
	a := discoveryServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/music-history" {
			fmt.Fprint(w, `{"result":{"historyTabs":[{"date":"2026-11-16","items":[`)
			for g := 0; g < 7; g++ {
				if g > 0 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprint(w, `{"tracks":[`)
				for i := 0; i < 40; i++ {
					if i > 0 {
						fmt.Fprint(w, ",")
					}
					fmt.Fprintf(w, `{"type":"track","data":{"itemId":{"trackId":"%d","albumId":9}}}`, g*40+i)
				}
				fmt.Fprint(w, "]}")
			}
			fmt.Fprint(w, `] }]}}`)
			return
		}
		if r.URL.Path == "/tracks" {
			r.ParseForm()
			ids := strings.Split(r.Form.Get("track-ids"), ",")
			if len(ids) != 40 || ids[0] != "40:9" {
				t.Error(ids)
			}
			fmt.Fprint(w, `{"result":[{"id":40,"title":"Song"}]}`)
			return
		}
		t.Error(r.URL)
	})
	refs, e := a.History(context.Background())
	if e != nil || len(refs) != 200 || refs[199].ID != "199" {
		t.Fatal(len(refs), e)
	}
	tracks, more, e := a.HistoryTracks(context.Background(), refs, 1)
	if e != nil || !more || len(tracks) != 1 {
		t.Fatal(tracks, more, e)
	}
}
func TestPageDecoderRetainsOnlyPage(t *testing.T) {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < 1000; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"%d","title":"Song"}`, i)
	}
	b.WriteByte(']')
	var p pageItems[Track]
	if e := json.Unmarshal([]byte(b.String()), &p); e != nil || len(p) != 40 || cap(p) > 80 {
		t.Fatal(len(p), cap(p), e)
	}
}
func BenchmarkSearchPageDecode(b *testing.B) {
	raw := []byte(`{"result":{"tracks":{"total":2,"perPage":40,"results":[{"id":1,"title":"One","artists":[{"id":2,"name":"Artist"}]},{"id":3,"title":"Two"}]}}}`)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out struct {
			Tracks searchGroup[Track] `json:"tracks"`
		}
		if e := decodeAPIResponse(strings.NewReader(string(raw)), false, &out); e != nil {
			b.Fatal(e)
		}
	}
}
