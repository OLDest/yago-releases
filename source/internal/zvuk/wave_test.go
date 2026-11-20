package zvuk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"yago/internal/music"
)

func TestWaveOptionsFeedbackAndDedup(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var in struct{ Variables map[string]json.RawMessage }
		if e := json.NewDecoder(r.Body).Decode(&in); e != nil {
			t.Fatal(e)
		}
		var opt map[string]any
		_ = json.Unmarshal(in.Variables["options"], &opt)
		if opt["mood"] != "energy:1,fun:0" || opt["language"] != "russian" || opt["vocal"] != 0.0 {
			t.Error(opt)
		}
		if calls == 1 || calls == 3 {
			if _, ok := in.Variables["contentInput"]; ok {
				t.Error("prefetch fabricated or repeated feedback")
			}
		} else {
			var p waveFeedback
			_ = json.Unmarshal(in.Variables["contentInput"], &p)
			if p.TrackID != "12" || p.PlayDuration != 30 || p.TrackDuration != 120 || !p.IsSkipped {
				t.Error(p)
			}
		}
		fmt.Fprint(w, `{"data":{"personalWaveContent":[{"id":"11","title":"old","duration":120},{"id":"12","title":"new","duration":120},{"id":"12","title":"repeat","duration":120}]}}`)
	})
	ctx := context.Background()
	seeds := []string{"zvuk:energy:1", "zvuk:fun:0", "zvuk:language:russian", "zvuk:vocal:0"}
	for i := 0; i < 3; i++ {
		if i == 1 {
			_ = c.Feedback(ctx, "", "", "skip", music.Track{ID: "zvuk-12", DurationMS: 120000}, 30)
		}
		w, e := c.WaveWithSeeds(ctx, "", []string{"zvuk-11"}, seeds)
		if e != nil || len(w.Sequence) != 1 || w.Sequence[0].Track.ID != "zvuk-12" || w.Session == "" {
			t.Fatal(w, e)
		}
	}
}

func TestWaveFailedFetchKeepsRealFeedback(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		var in struct {
			Variables struct{ ContentInput *waveFeedback }
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Variables.ContentInput == nil {
			t.Error("lost feedback")
		}
		fmt.Fprint(w, `{"data":{"personalWaveContent":[]}}`)
	})
	_ = c.Feedback(context.Background(), "", "", "trackFinished", music.Track{ID: "zvuk-12", DurationMS: 1000}, 4)
	if _, e := c.WaveWithSeeds(context.Background(), "", nil, nil); e == nil {
		t.Fatal("expected failure")
	}
	if _, e := c.WaveWithSeeds(context.Background(), "", nil, nil); e != nil {
		t.Fatal(e)
	}
}
