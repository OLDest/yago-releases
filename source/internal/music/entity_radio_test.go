package music

import (
	"context"
	"testing"
)

func TestEntityRadioInAllDoesNotAddUnrelatedPersonalWave(t *testing.T) {
	for _, seed := range []string{"track:42", "artist:42", "album:42", "zvuk:radio:PLAYLIST:42"} {
		y, z := &waveProbe{}, &waveProbe{prefix: "zvuk-"}
		s := &Services{Yandex: &API{Backend: y}, Zvuk: &API{Backend: z}}
		w, e := s.WaveWithSeeds(context.Background(), "", nil, []string{seed})
		if e != nil || len(w.Sequence) != 5 {
			t.Fatal(e)
		}
		wantZ := len(seed) > 5 && seed[:5] == "zvuk:"
		if wantZ && (y.calls != 0 || z.calls != 1) || !wantZ && (y.calls != 1 || z.calls != 0) {
			t.Fatal("unrelated source called", seed)
		}
	}
}
