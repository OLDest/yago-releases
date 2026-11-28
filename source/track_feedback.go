package main

import (
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"strings"
	"yago/internal/music"
)

type trackFeedbackUI struct {
	likes, blocks []widget.Clickable
	liked         map[music.ID]bool
}

var rowBlockIcon = feedbackIcon(icons.ContentBlock)
var rowLikeIcon = feedbackIcon(icons.ActionFavoriteBorder)
var rowLikedIcon = feedbackIcon(icons.ActionFavorite)

func feedbackIcon(data []byte) *widget.Icon {
	icon, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	return icon
}

func (u *UI) trackLiked(t music.Track) bool {
	if liked, known := u.trackFeedback.liked[t.ID]; known {
		return liked
	}
	if !u.isOfflineView() && u.disc.view.Kind == "likes" {
		for _, visible := range u.tracks {
			if visible.ID == t.ID {
				return true
			}
		}
	}
	return false
}

// Bind feedback to the track shown by Android, not whatever starts before the
// queued callback reaches the UI. No network work runs on the service thread.
func (u *UI) notificationFeedback(command string) bool {
	action, id, _ := strings.Cut(command, ":")
	if action != "like" && action != "dislike" {
		return false
	}
	if id == "" || music.ID(id) != u.current.ID || u.api == nil {
		return true
	}
	if action == "like" {
		u.likeTrack(u.current)
	} else {
		u.moderateTrack(menuEntry{Action: "dislike", ID: u.current.ID}, u.current)
	}
	return true
}

func (u *UI) markTrackLiked(id music.ID, liked bool) {
	if u.trackFeedback.liked == nil {
		u.trackFeedback.liked = make(map[music.ID]bool)
	}
	u.trackFeedback.liked[id] = liked
}

func (u *UI) likeTrack(t music.Track) {
	if t.ID == "" {
		return
	}
	if u.blocked(t) {
		u.status = "Сначала снимите блокировку трека или исполнителя"
		return
	}
	action := "like"
	if u.trackLiked(t) {
		action = "unlike"
	}
	u.moderateTrack(menuEntry{Action: action, ID: t.ID}, t)
}

func (u *UI) trackFeedbackInputs(gtx layout.Context) {
	if u.page == "playlists" || u.disc.menu != "" {
		return
	}
	for i := range u.trackFeedback.blocks {
		if i >= len(u.tracks) {
			break
		}
		if u.trackFeedback.blocks[i].Clicked(gtx) {
			t := u.tracks[i]
			action := "dislike"
			if u.disc.prefs.Tracks[t.ID] {
				action = "allow-track"
			}
			u.moderateTrack(menuEntry{Action: action, ID: t.ID}, t)
			return
		}
		if u.trackFeedback.likes[i].Clicked(gtx) {
			u.likeTrack(u.tracks[i])
			return
		}
	}
}

// These controls are siblings of the playback hit area, never its children.
func (u *UI) trackRowActions(gtx layout.Context, i int) layout.Dimensions {
	if u.page == "playlists" || i >= len(u.tracks) || i >= len(u.trackFeedback.likes) {
		return layout.Dimensions{}
	}
	t := u.tracks[i]
	button := func(gtx layout.Context, click *widget.Clickable, icon *widget.Icon, label string, selected bool) layout.Dimensions {
		v := material.IconButton(u.theme, click, icon, label)
		v.Size = 20
		v.Inset = layout.UniformInset(10)
		v.Background = panel
		v.Color = white
		if selected {
			v.Color = accent
		}
		return v.Layout(gtx)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := "Не рекомендовать этот трек"
			if u.disc.prefs.Tracks[t.ID] {
				label = "Разрешить этот трек"
			}
			return button(gtx, &u.trackFeedback.blocks[i], rowBlockIcon, label, u.disc.prefs.Tracks[t.ID])
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.offlineRowAction(gtx, i) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			liked := u.trackLiked(t)
			icon := rowLikeIcon
			if liked {
				icon = rowLikedIcon
			}
			return button(gtx, &u.trackFeedback.likes[i], icon, "Добавить в «Мне нравится»", liked)
		}))
}
