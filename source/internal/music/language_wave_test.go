package music

import (
	"context"
	"testing"
)

func TestUnavailableLanguageExcludesZvukWithoutSendingMarker(t *testing.T) {
	y, z := &waveProbe{}, &waveProbe{prefix: "zvuk-"}
	s := &Services{Yandex: &API{Backend: y}, Zvuk: &API{Backend: z}}
	w, e := s.WaveWithSeeds(context.Background(), "", nil, []string{"settingLanguage:not-russian", ExcludeZvukWave, "zvuk:language:all"})
	if e != nil || len(w.Sequence) != 5 || z.calls != 0 || len(y.seeds) != 1 || y.seeds[0] != "settingLanguage:not-russian" {
		t.Fatal("unsupported language mixed or marker leaked", e)
	}
}
