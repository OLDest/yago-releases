package main

import (
	"time"
	"yago/internal/music"
	"yago/internal/store"
)

func (u *UI) recordPlayedHistory() {
	if u.player == nil || u.current.ID == "" || u.historyPlayID == u.playID {
		return
	}
	s := u.player.Snapshot()
	if s.ID != u.playID || !s.Playing || s.Position < .5 {
		return
	}
	owner := u.historyOwner(u.current.ID)
	if owner == "" {
		return
	}
	u.historyPlayID = u.playID
	_ = store.RecordHistory(stateDir, owner, u.current, time.Now())
}
func (u *UI) readLocalHistory() []music.HistoryRef {
	var out []music.HistoryRef
	for _, owner := range u.historyOwners() {
		items, e := store.LoadHistory(stateDir, owner)
		if e != nil {
			continue
		}
		var part []music.HistoryRef
		for _, h := range items {
			ref := music.HistoryRef{ID: h.Track.ID, Date: h.At.Format(time.RFC3339Nano)}
			if len(h.Track.Albums) > 0 {
				ref.AlbumID = h.Track.Albums[0].ID
			}
			part = append(part, ref)
		}
		out = music.MergeHistory(out, part, 400)
	}
	return out
}
