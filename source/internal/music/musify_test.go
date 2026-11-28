package music

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"yago/internal/musify"
)

type musifyRoundTrip func(*http.Request) (*http.Response, error)

func (f musifyRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func musifyResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestMusifySearchChartsAndStreamContract(t *testing.T) {
	calls := 0
	client := musify.New(&http.Client{Transport: musifyRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("X-Install-Id") != "local-install-id" {
			t.Fatal("missing installation ID")
		}
		switch r.URL.Path {
		case "/api/mobile/v1/search":
			if r.URL.Query().Get("q") != "Beatles" || r.URL.Query().Get("type") != "all" || r.URL.Query().Get("page") != "2" {
				t.Fatal("wrong search request", r.URL)
			}
			return musifyResponse(200, `{"songs":[{"id":42,"title":"Song","artist":"Artist","durationSec":123}],"artists":[{"id":3,"name":"Artist"}],"albums":[{"id":4,"title":"Album"}],"hasMore":true}`), nil
		case "/api/mobile/v1/charts":
			return musifyResponse(200, `{"entries":[{"rank":1,"song":{"id":42,"title":"Song","artist":"Artist","durationSec":123}}]}`), nil
		case "/api/mobile/v1/track/42/stream-url":
			return musifyResponse(200, `{"url":"https://media.example.test/song.mp3"}`), nil
		default:
			t.Fatal("unexpected request", r.URL)
			return nil, nil
		}
	})}, "local-install-id")
	provider := &Musify{Client: client}
	ctx := context.Background()
	search, err := provider.SearchPage(ctx, "Beatles", "all", 1)
	if err != nil || !search.More || len(search.Tracks) != 1 || search.Tracks[0].ID != "musify-42" || search.Tracks[0].DurationMS != 123000 || len(search.Artists) != 1 || len(search.Albums) != 1 {
		t.Fatalf("search conversion: %+v, %v", search, err)
	}
	charts, err := provider.Charts(ctx)
	if err != nil || len(charts) != 1 || charts[0].ID != "musify-42" {
		t.Fatalf("chart conversion: %+v, %v", charts, err)
	}
	stream, err := provider.Stream(ctx, charts[0], High)
	if err != nil || stream.URL != "https://media.example.test/song.mp3" || calls != 3 {
		t.Fatalf("stream conversion: %+v, calls=%d, %v", stream, calls, err)
	}
}

func TestMusifyForbiddenDoesNotRetry(t *testing.T) {
	calls := 0
	client := musify.New(&http.Client{Transport: musifyRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return musifyResponse(403, "forbidden"), nil
	})}, "local-install-id")
	_, err := (&Musify{Client: client}).SearchPage(context.Background(), "Beatles", "all", 0)
	var httpErr *musify.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != 403 || calls != 1 {
		t.Fatalf("forbidden response: %v, calls=%d", err, calls)
	}
}

func TestMusifyFavoritesAndDownloadContract(t *testing.T) {
	calls := 0
	client := musify.New(&http.Client{Transport: musifyRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-session" {
			t.Fatal("missing session")
		}
		switch r.Method + " " + r.URL.Path {
		case "PUT /api/mobile/v1/me/favorites/tracks/42", "DELETE /api/mobile/v1/me/favorites/tracks/42":
			return musifyResponse(204, ""), nil
		case "GET /api/mobile/v1/me/favorites/tracks":
			if r.URL.Query().Get("page") != "0" {
				t.Fatal("favorites must start at page zero")
			}
			return musifyResponse(200, `{"items":[{"id":42,"title":"test"}],"totalCount":1}`), nil
		case "GET /api/mobile/v1/track/42/file-url":
			return musifyResponse(200, `{"url":"https://example.test/audio"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})}, "test-install")
	client.SetSession("test-session")
	provider := &Musify{Client: client}
	ctx := context.Background()
	track := Track{ID: "musify-42"}
	if err := provider.Like(ctx, "", track); err != nil {
		t.Fatal(err)
	}
	tracks, err := provider.Likes(ctx, "")
	if err != nil || len(tracks) != 1 || tracks[0].ID != track.ID {
		t.Fatalf("favorites: %v %v", tracks, err)
	}
	if err = provider.Unlike(ctx, "", track); err != nil {
		t.Fatal(err)
	}
	info, err := provider.Download(ctx, track, 0)
	if err != nil || info.URL != "https://example.test/audio" || calls != 4 {
		t.Fatalf("download result: %v, calls=%d", err, calls)
	}
}

func TestAllWithOnlyMusifyIncludesChartsAndPlaylists(t *testing.T) {
	client := musify.New(&http.Client{Transport: musifyRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/mobile/v1/charts/full":
			if r.URL.Query().Get("cursor") == "page-two" {
				return musifyResponse(200, `{"entries":[{"rank":3,"song":{"id":44,"title":"Second page"}}]}`), nil
			}
			return musifyResponse(200, `{"nextCursor":"page-two","entries":[{"rank":1,"song":{"id":42,"title":"Chart track"}},{"rank":2,"song":{"id":43,"title":"Next track"}}]}`), nil
		case "/api/mobile/v1/me/playlists":
			return musifyResponse(200, `{"items":[{"id":71,"name":"Playlist"}]}`), nil
		default:
			t.Fatalf("unexpected route %s", r.URL.Path)
			return nil, nil
		}
	})}, "test-install")
	client.SetSession("test-session")
	services := &Services{Musify: &API{Backend: &Musify{Client: client}}}
	ctx := context.Background()
	wave, err := services.WaveWithSeeds(ctx, "", []string{"musify-42"}, nil)
	if err != nil || len(wave.Sequence) != 1 || wave.Sequence[0].Track.ID != "musify-43" {
		t.Fatalf("wave: %v, %v", wave, err)
	}
	next, err := services.WaveWithSeeds(ctx, wave.Session, []string{"musify-42", "musify-43"}, nil)
	if err != nil || len(next.Sequence) != 1 || next.Sequence[0].Track.ID != "musify-44" {
		t.Fatalf("refill: %v, %v", next, err)
	}
	lists, err := services.Playlists(ctx, "")
	if err != nil || len(lists) != 1 || lists[0].Kind != "musify-71" {
		t.Fatalf("playlists: %v, %v", lists, err)
	}
}

func TestMusifyHistoryAndPlaylistWrites(t *testing.T) {
	steps := []string{"GET /api/mobile/v1/me/history", "GET /api/mobile/v1/me/history", "POST /api/mobile/v1/me/playlists", "PATCH /api/mobile/v1/me/playlists/71", "POST /api/mobile/v1/me/playlists/71/tracks", "DELETE /api/mobile/v1/me/playlists/71/tracks/42", "DELETE /api/mobile/v1/me/playlists/71"}
	calls := 0
	client := musify.New(&http.Client{Transport: musifyRoundTrip(func(r *http.Request) (*http.Response, error) {
		if calls >= len(steps) || r.Method+" "+r.URL.Path != steps[calls] {
			t.Fatalf("bad step %d: %s %s", calls, r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-session" {
			t.Fatal("missing session")
		}
		calls++
		switch calls {
		case 1:
			return musifyResponse(200, `{"items":[{"song":{"id":42,"title":"One"},"playedAt":"2026-11-28T18:00:00Z"}],"cursor":"next"}`), nil
		case 2:
			if r.URL.Query().Get("cursor") != "next" {
				t.Fatal("missing history cursor")
			}
			return musifyResponse(200, `{"items":[{"song":{"id":43,"title":"Two"},"playedAt":"2026-11-28T17:00:00Z"}]}`), nil
		case 3, 4, 5:
			body, _ := io.ReadAll(r.Body)
			want := []string{`{"description":"","name":"QA"}`, `{"name":"QA"}`, `{"songId":42}`}[calls-3]
			if string(body) != want || r.Header.Get("Content-Type") != "application/json" {
				t.Fatal("incorrect playlist payload")
			}
		}
		return musifyResponse(204, ""), nil
	})}, "install")
	client.SetSession("test-session")
	provider := &Musify{Client: client}
	all := &Services{Musify: &API{Backend: provider}}
	ctx := context.Background()
	refs, err := all.History(ctx)
	if err != nil || len(refs) != 2 {
		t.Fatalf("history: %v %v", refs, err)
	}
	tracks, more, err := all.HistoryTracks(ctx, refs, 0)
	if err != nil || more || len(tracks) != 2 || tracks[0].ID != "musify-42" {
		t.Fatalf("history tracks: %v %v", tracks, err)
	}
	for _, op := range []string{"create", "rename", "add", "remove", "delete"} {
		if err := client.EditPlaylist(ctx, op, 71, 42, "QA"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != len(steps) {
		t.Fatalf("got %d calls", calls)
	}
}

func TestMusifyChartFiltersUseMusifyFeeds(t *testing.T) {
	client := musify.New(&http.Client{Transport: musifyRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/mobile/v1/charts/tracks":
			if r.URL.Query().Get("period") != "week" {
				t.Fatal("wrong period")
			}
			return musifyResponse(200, `{"items":[{"id":1}],"nextCursor":"next"}`), nil
		case "/api/mobile/v1/genre/11/top":
			return musifyResponse(200, `{"items":[{"id":2}]}`), nil
		case "/api/mobile/v1/tag/3/albums":
			return musifyResponse(200, `{"items":[{"id":12}]}`), nil
		case "/api/mobile/v1/album/12":
			return musifyResponse(200, `{"tracks":[{"id":3}]}`), nil
		default:
			t.Fatalf("unexpected route %s", r.URL.Path)
			return nil, nil
		}
	})}, "install")
	for _, item := range []struct {
		source string
		song   int64
		cursor string
	}{{"week", 1, "next"}, {"genre:11", 2, ""}, {"mood:3", 3, ""}} {
		page, cursor, err := client.ChartPage(context.Background(), item.source, "")
		if err != nil || cursor != item.cursor || len(page.Entries) != 1 || page.Entries[0].Song.ID != item.song {
			t.Fatalf("%s: wrong chart: %v, %q, %v", item.source, page, cursor, err)
		}
	}
	if musify.ValidChartSource("spotify") {
		t.Fatal("external chart still selectable")
	}
}

func TestMusifyMixInterleavesGenreAndPublicPlaylists(t *testing.T) {
	client := musify.New(&http.Client{Transport: musifyRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/mobile/v1/genre/11/top":
			if r.URL.Query().Get("cursor") == "g2" {
				return musifyResponse(200, `{"items":[{"id":4}]}`), nil
			}
			return musifyResponse(200, `{"items":[{"id":1},{"id":2}],"nextCursor":"g2"}`), nil
		case "/api/mobile/v1/theme/11":
			return musifyResponse(200, `{"playlists":[{"id":71},{"id":72}]}`), nil
		case "/api/mobile/v1/me/playlists/71":
			return musifyResponse(200, `{"tracks":[{"id":2},{"id":3}]}`), nil
		case "/api/mobile/v1/me/playlists/72":
			return musifyResponse(200, `{"tracks":[{"id":5}]}`), nil
		default:
			t.Fatalf("unexpected route %s", r.URL.Path)
			return nil, nil
		}
	})}, "install")
	source := "genre:11,theme:11"
	page, next, err := client.ChartPage(context.Background(), source, "")
	if err != nil || next == "" || len(page.Entries) != 3 || page.Entries[0].Song.ID != 1 || page.Entries[1].Song.ID != 2 || page.Entries[2].Song.ID != 3 {
		t.Fatalf("first mix: %v %q %v", page, next, err)
	}
	page, next, err = client.ChartPage(context.Background(), source, next)
	if err != nil || next != "" || len(page.Entries) != 2 || page.Entries[0].Song.ID != 4 || page.Entries[1].Song.ID != 5 {
		t.Fatalf("next mix: %v %q %v", page, next, err)
	}
}
