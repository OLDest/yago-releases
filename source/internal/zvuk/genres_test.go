package zvuk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestGenreWaveNativePayloadAndReset(t *testing.T) {
	var wantName, wantLevel string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Variables struct {
				Options struct{ Genre []struct{ Name, Type string } }
			}
		}
		if e := json.NewDecoder(r.Body).Decode(&in); e != nil {
			t.Fatal(e)
		}
		g := in.Variables.Options.Genre
		if wantName == "" {
			if len(g) != 0 {
				t.Error("reset retained genre")
			}
		} else if len(g) != 1 || g[0].Name != wantName || g[0].Type != wantLevel {
			t.Error("incorrect native genre", g)
		}
		fmt.Fprint(w, `{"data":{"personalWaveContent":[{"id":"1","title":"one"}]}}`)
	})
	for _, tc := range []struct{ seed, name, level string }{{"zvuk:genre:rock", "rock", "LVL1"}, {"zvuk:genre:chanson", "chanson", "LVL2"}, {"zvuk:genre:russian_rock", "russian_rock", "LVL3"}, {"zvuk:genre:any", "", ""}, {"zvuk:genre:unknown", "", ""}} {
		wantName, wantLevel = tc.name, tc.level
		if _, e := c.WaveWithSeeds(context.Background(), "", nil, []string{tc.seed}); e != nil {
			t.Fatal(e)
		}
	}
}
