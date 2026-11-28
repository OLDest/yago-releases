package zvuk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestRadioCursorAndIndependentNumericControls(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var in struct {
			Variables struct {
				Cursor  int
				Type    string
				Options struct {
					Mood           string
					Popular, Vocal float64
					Genre          []struct{ Name, Type string }
				}
			}
		}
		if e := json.NewDecoder(r.Body).Decode(&in); e != nil {
			t.Fatal(e)
		}
		if calls < 3 {
			if in.Variables.Type != "PLAYLIST" || in.Variables.Cursor != (calls-1)*5 {
				t.Error("cursor/type", in.Variables)
			}
			fmt.Fprintf(w, `{"data":{"recommenderRadio":{"cursor":%d,"tracks":[{"id":"%d","title":"one"}]}}}`, calls*5, calls)
			return
		}
		o := in.Variables.Options
		if o.Mood != "energy:0.3,fun:0.7" || o.Popular != 0.2 || o.Vocal != 0.5 || len(o.Genre) != 2 {
			t.Error("numeric/multi settings", o)
		}
		fmt.Fprint(w, `{"data":{"personalWaveContent":[{"id":"3"}]}}`)
	})
	ctx := context.Background()
	w, e := c.WaveWithSeeds(ctx, "", nil, []string{"zvuk:radio:PLAYLIST:42"})
	if e != nil || len(w.Sequence) != 1 {
		t.Fatal(e)
	}
	next, e := c.WaveWithSeeds(ctx, w.Session, []string{"zvuk-1"}, []string{"zvuk:radio:PLAYLIST:42"})
	if e != nil || len(next.Sequence) != 1 || next.Session == w.Session {
		t.Fatal("radio continuation", e)
	}
	_, e = c.WaveWithSeeds(ctx, "", nil, []string{"zvuk:energy:0.3", "zvuk:fun:0.7", "zvuk:popular:0.2", "zvuk:vocal:0.5", "zvuk:genre:rock", "zvuk:genre:jazz"})
	if e != nil {
		t.Fatal(e)
	}
}
