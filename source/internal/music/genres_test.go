package music

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestGenreOptionsBoundedAndDeduplicated(t *testing.T) {
	a := discoveryServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/genres" {
			t.Error(r.URL.Path)
		}
		fmt.Fprint(w, `{"result":[{"id":"all","title":"Все"},{"id":"rock","title":"Рок","subGenres":[{"id":"rock","title":"Повтор"},{"id":"hardrock","title":"Хард-рок"}`)
		for i := 0; i < 3000; i++ {
			fmt.Fprintf(w, `,{"id":"g%d","title":"Genre"}`, i)
		}
		fmt.Fprint(w, `]},{"id":"pop","title":"Поп"}]}`)
	})
	o := a.genreWaveOptions(context.Background())
	if len(o) != 2048 || o[1].Seed != "genre:rock" || o[2].Seed != "genre:pop" || o[3].Seed != "genre:hardrock" {
		t.Fatal("bad genre catalog", len(o))
	}
}
