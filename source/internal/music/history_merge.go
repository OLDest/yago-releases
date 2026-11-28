package music

import (
	"sort"
	"time"
)

// Yandex exposes calendar days, Zvuk exact timestamps. Interleave same-day
// entries rather than inventing a time for Yandex and pushing an entire provider behind it.
func MergeHistory(a, b []HistoryRef, limit int) []HistoryRef {
	if limit <= 0 {
		return nil
	}
	refs := interleave(a, b)
	day := func(s string) string {
		if len(s) == 10 {
			if _, e := time.Parse("2006-01-02", s); e == nil {
				return s
			}
		}
		for _, format := range []string{time.RFC3339Nano, "2006-01-02"} {
			if t, e := time.Parse(format, s); e == nil {
				return t.Local().Format("2006-01-02")
			}
		}
		return ""
	}
	type dated struct {
		ref HistoryRef
		day string
	}
	entries := make([]dated, len(refs))
	for i, r := range refs {
		entries[i] = dated{r, day(r.Date)}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].day > entries[j].day })
	out := make([]HistoryRef, 0, min(limit, len(refs)))
	seen := map[ID]bool{}
	for _, entry := range entries {
		r := entry.ref
		if r.ID != "" && !seen[r.ID] {
			seen[r.ID] = true
			out = append(out, r)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}
