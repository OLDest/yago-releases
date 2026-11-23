package music

import "context"

// Backend shares the compact player models without bringing another runtime into the app.
type Backend interface {
	Account(context.Context) (Account, error)
	Search(context.Context, string) ([]Track, error)
	SearchPage(context.Context, string, string, int) (SearchPage, error)
	Tracks(context.Context, []string) ([]Track, error)
	Likes(context.Context, ID) ([]Track, error)
	Playlists(context.Context, ID) ([]Playlist, error)
	Playlist(context.Context, ID, ID) ([]Track, error)
	Like(context.Context, ID, Track) error
	Unlike(context.Context, ID, Track) error
	Dislike(context.Context, ID, string, ID, bool) error
	Stream(context.Context, Track, Quality) (StreamInfo, error)
	AlbumPage(context.Context, ID, int) ([]Track, bool, error)
	ArtistTracks(context.Context, ID, int) ([]Track, bool, error)
	History(context.Context) ([]HistoryRef, error)
	HistoryTracks(context.Context, []HistoryRef, int) ([]Track, bool, error)
	WaveWithSeeds(context.Context, string, []string, []string) (Wave, error)
	Feedback(context.Context, string, string, string, Track, float64) error
	WaveOptions(context.Context) ([]WaveOption, error)
	Suggest(context.Context, string) ([]string, error)
}
