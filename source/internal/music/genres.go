package music

import "context"

// Fetch only labels/IDs when settings are opened, never artwork or track lists.
func (a *API) genreWaveOptions(ctx context.Context) []WaveOption {
	type genre struct {
		ID, Title string
		Children  []genre `json:"subGenres"`
	}
	var genres []genre
	if a.callBounded(ctx, "GET", "/genres", nil, nil, &genres, discoveryLimit) != nil {
		return nil
	}
	out := []WaveOption{{Name: "Любой", Seed: "user:onyourwave", Group: "genre"}}
	seen := map[string]bool{"all": true}
	add := func(id, name string) {
		if id == "" || name == "" || len(id) > 80 || seen[id] || len(out) >= 2048 {
			return
		}
		seen[id] = true
		out = append(out, WaveOption{Name: name, Seed: "genre:" + id, Group: "genre"})
	}
	for _, g := range genres {
		add(g.ID, g.Title)
	}
	var children func([]genre, int)
	children = func(gs []genre, depth int) {
		if depth > 8 {
			return
		}
		for _, g := range gs {
			add(g.ID, g.Title)
			children(g.Children, depth+1)
		}
	}
	for _, g := range genres {
		children(g.Children, 0)
	}
	if len(out) == 1 {
		return nil
	}
	return out
}
