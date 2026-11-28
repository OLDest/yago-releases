package zvuk

import (
	"context"
	"errors"
	"yago/internal/music"
)

func (c *Client) LikesPage(ctx context.Context, _ music.ID, page int) ([]music.Track, bool, error) {
	if page < 0 || page > 1000 {
		return nil, false, errors.New("Неверная страница")
	}
	var data struct{ Collection struct{ Tracks []named } }
	e := c.gql(ctx, `query{collection{tracks(orderBy:dateAdded,orderDirection:desc){id}}}`, nil, &data)
	if e != nil {
		return nil, false, e
	}
	start := min(page*music.PageSize, len(data.Collection.Tracks))
	end := min(start+music.PageSize, len(data.Collection.Tracks))
	ids := make([]string, 0, end-start)
	for _, r := range data.Collection.Tracks[start:end] {
		ids = append(ids, string(ns(r.ID)))
	}
	ts, e := c.Tracks(ctx, ids)
	return ts, end < len(data.Collection.Tracks), e
}
