package zvuk

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"yago/internal/music"
)

type waveFeedback struct {
	TrackID       string `json:"trackId"`
	TrackDuration int    `json:"trackDuration"`
	PlayDuration  int    `json:"playDuration"`
	IsSkipped     bool   `json:"isSkipped"`
}

func (c *Client) WaveWithSeeds(ctx context.Context, session string, queue, seeds []string) (music.Wave, error) {
	for _, s := range seeds {
		if strings.HasPrefix(s, "zvuk:radio:") {
			return c.entityRadio(ctx, s, session, queue)
		}
	}
	options := map[string]any{"mood": "energy:0.5,fun:0.5", "popular": nil, "vocal": 1.0}
	energy, fun := "0.5", "0.5"
	var genres []map[string]string
	for _, seed := range seeds {
		if genre := waveGenre(seed); genre != nil {
			genres = append(genres, genre...)
		}
		for _, group := range []string{"energy", "fun", "popular", "vocal"} {
			if raw, ok := strings.CutPrefix(seed, "zvuk:"+group+":"); ok {
				if v, e := strconv.ParseFloat(raw, 64); e == nil && v >= 0 && v <= 1 {
					switch group {
					case "energy":
						energy = strconv.FormatFloat(v, 'f', -1, 64)
					case "fun":
						fun = strconv.FormatFloat(v, 'f', -1, 64)
					default:
						options[group] = v
					}
				}
			}
		}
		if seed == "zvuk:language:russian" {
			options["language"] = "russian"
		}

	}
	if len(genres) > 0 {
		options["genre"] = genres[:min(music.MaxWaveGenres, len(genres))]
	}
	options["mood"] = "energy:" + energy + ",fun:" + fun
	c.mu.Lock()
	pending := c.waveFeedback
	c.mu.Unlock()
	vars := map[string]any{"first": 5, "waveSrc": "AMAZME", "options": options}
	if pending != nil {
		vars["contentInput"] = pending
	}
	var data struct {
		Tracks []track `json:"personalWaveContent"`
	}
	err := c.gql(ctx, `query($contentInput:PersonalWaveContentInput,$first:PositiveInt!,$options:PersonalWaveOptions,$waveSrc:MagicSource){personalWaveContent(contentInput:$contentInput,first:$first,options:$options,waveSrc:$waveSrc){... on Track{`+trackFields+`}}}`, vars, &data)
	if err != nil {
		return music.Wave{}, err
	}
	c.mu.Lock()
	if c.waveFeedback == pending {
		c.waveFeedback = nil
	}
	c.waveSerial++
	serial := c.waveSerial
	c.mu.Unlock()
	if !strings.HasPrefix(session, "zvuk-wave:") {
		session = fmt.Sprintf("zvuk-wave:%d", serial)
	}
	w := music.Wave{Session: session, Batch: fmt.Sprint(serial)}
	seen := make(map[string]bool, len(queue))
	for _, id := range queue {
		seen[id] = true
	}
	for _, t := range models(data.Tracks[:min(5, len(data.Tracks))]) {
		if t.ID != "" && !seen[string(t.ID)] {
			w.Sequence = append(w.Sequence, struct {
				Track music.Track `json:"track"`
			}{t})
			seen[string(t.ID)] = true
		}
	}
	return w, nil
}

// Only actual playback supplies feedback. Prefetch never fabricates listening.
// One pending record bounds memory; a failed request keeps it for retry.
func (c *Client) Feedback(_ context.Context, session, _, kind string, t music.Track, played float64) error {
	if strings.HasPrefix(session, "zvuk-radio:") {
		return nil
	}
	if kind != "skip" && kind != "trackFinished" {
		return nil
	}
	id, err := ID(t.ID)
	if err != nil {
		return err
	}
	duration := int(t.DurationMS / 1000)
	if duration <= 0 || played <= 0 {
		return nil
	}
	p := &waveFeedback{TrackID: id, TrackDuration: duration, PlayDuration: min(duration, int(played)), IsSkipped: kind == "skip"}
	c.mu.Lock()
	c.waveFeedback = p
	c.mu.Unlock()
	return nil
}
func (c *Client) WaveOptions(context.Context) ([]music.WaveOption, error) {
	var out []music.WaveOption
	for _, g := range []struct{ key, name string }{{"popular", "Популярность"}, {"energy", "Энергия"}, {"fun", "Настроение"}, {"vocal", "Вокал"}} {
		for n := 0; n <= 10; n++ {
			v := float64(n) / 10
			label := fmt.Sprintf("%s: %d%%", g.name, n*10)
			if n == 0 {
				label += " · " + map[string]string{"popular": "открытия", "energy": "спокойно", "fun": "грустно", "vocal": "без вокала"}[g.key]
			}
			if n == 10 {
				label += " · " + map[string]string{"popular": "хиты", "energy": "бодро", "fun": "весело", "vocal": "с вокалом"}[g.key]
			}
			if n == 5 {
				label += " · баланс"
			}
			out = append(out, music.WaveOption{Name: label, Seed: "zvuk:" + g.key + ":" + strconv.FormatFloat(v, 'f', -1, 64), Group: g.key})
		}
	}
	out = append(out, music.WaveOption{Name: "Язык: любой", Seed: "zvuk:language:all", Group: "language"}, music.WaveOption{Name: "Язык: русский", Seed: "zvuk:language:russian", Group: "language"})
	return append(out, genreOptions()...), nil
}
