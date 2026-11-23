package zvuk

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"yago/internal/music"
)

func (c *Client) entityRadio(ctx context.Context, seed, session string, queue []string) (music.Wave, error) {
	parts := strings.Split(strings.TrimPrefix(seed, "zvuk:radio:"), ":")
	if len(parts) != 2 || !map[string]bool{"TRACK": true, "ARTIST": true, "RELEASE": true, "PLAYLIST": true}[parts[0]] {
		return music.Wave{}, errors.New("Неверный источник радио")
	}
	if _, e := ID(music.ID("zvuk-" + parts[1])); e != nil {
		return music.Wave{}, e
	}
	prefix := "zvuk-radio:" + parts[0] + ":" + parts[1] + ":"
	cursor := 0
	if raw, ok := strings.CutPrefix(session, prefix); ok {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 0 {
			return music.Wave{}, errors.New("Неверная страница радио")
		}
		cursor = n
	}
	var data struct {
		Radio struct {
			Cursor music.ID
			Tracks []track
		} `json:"recommenderRadio"`
	}
	vars := map[string]any{"id": parts[1], "type": parts[0], "first": 5, "cursor": cursor}
	if e := c.gql(ctx, `query($id:ID!,$type:RecommenderRadioEntityType!,$first:NonNegativeInt!,$cursor:NonNegativeInt){recommenderRadio(onEntity:{id:$id,type:$type},first:$first,cursor:$cursor){cursor tracks{`+trackFields+`}}}`, vars, &data); e != nil {
		return music.Wave{}, e
	}
	next, e := strconv.Atoi(string(data.Radio.Cursor))
	if e != nil || next <= cursor {
		next = cursor + len(data.Radio.Tracks)
	}
	w := music.Wave{Session: prefix + strconv.Itoa(next), Batch: strconv.Itoa(cursor)}
	seen := map[string]bool{}
	for _, id := range queue {
		seen[id] = true
	}
	for _, t := range models(data.Radio.Tracks[:min(5, len(data.Radio.Tracks))]) {
		if t.ID != "" && !seen[string(t.ID)] {
			w.Sequence = append(w.Sequence, struct {
				Track music.Track `json:"track"`
			}{t})
			seen[string(t.ID)] = true
		}
	}
	return w, nil
}
