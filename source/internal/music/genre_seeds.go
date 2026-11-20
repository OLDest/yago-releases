package music

import "strings"

const MaxWaveGenres = 32

// One bounded preference string retains compatibility with existing profiles.
func GenreSeeds(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range strings.Split(raw, "|") {
		if len(out) >= MaxWaveGenres {
			break
		}
		if (strings.HasPrefix(s, "genre:") || strings.HasPrefix(s, "zvuk:genre:")) && !strings.HasSuffix(s, ":any") && len(s) <= 100 && !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	return out
}
