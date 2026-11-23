package music

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type allocationTransport struct{ body string }

func (t allocationTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(t.body))}, nil
}
func BenchmarkLargePlaylistResponse(b *testing.B) {
	tracks := make([]map[string]any, 5000)
	for i := range tracks {
		tracks[i] = map[string]any{"track": map[string]any{"id": i + 1, "title": "Synthetic track title for memory audit", "artists": []map[string]any{{"id": 1, "name": "Synthetic artist"}}, "durationMs": 180000}}
	}
	raw, e := json.Marshal(map[string]any{"result": map[string]any{"tracks": tracks}})
	if e != nil {
		b.Fatal(e)
	}
	a := New("")
	a.HTTP = &http.Client{Transport: allocationTransport{string(raw)}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracks, e := a.Playlist(context.Background(), "1", "2")
		if e != nil || len(tracks) != 5000 {
			b.Fatal(e, len(tracks))
		}
	}
}
