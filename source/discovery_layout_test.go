package main

import (
	"context"
	"fmt"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"yago/internal/audio"
	"yago/internal/music"
	"yago/internal/updates"
)

func TestDiscoveryPanelsLayout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &UI{ctx: ctx, theme: newUITheme(), player: audio.New(t.TempDir(), nil), account: music.Account{UID: "test", DisplayName: "Демо"}, api: music.New(""), title: "Результаты поиска", page: "tracks", current: music.Track{ID: "song", Title: "Проверка длинного названия композиции", Artists: []music.Named{{ID: "artist", Name: "Исполнитель"}}, DurationMS: 180000}}
	defer u.player.Close()
	u.theme.Palette.Bg = bg
	u.theme.Palette.Fg = white
	u.theme.Palette.ContrastBg = accent
	u.theme.Palette.ContrastFg = bg
	u.list.Axis = layout.Vertical
	u.editor.SingleLine = true
	u.editor.SetText("Музыка")
	u.setTracks([]music.Track{u.current})
	u.disc.view = exploreView{Kind: "search", More: true}
	out := os.Getenv("YAGO_DISCOVERY_RENDER_DIR")
	for _, size := range []image.Point{image.Pt(320, 480), image.Pt(320, 640), image.Pt(380, 820), image.Pt(800, 480), image.Pt(1080, 740), image.Pt(1600, 829)} {
		for _, mode := range []string{"search", "search-mixed", "actions", "settings", "artists", "updates", "new-update"} {
			u.up.show = false
			u.up.available = nil
			u.closeMenu()
			u.page = "tracks"
			u.title = "Результаты поиска"
			switch mode {
			case "search-mixed":
				u.page = "discovery"
				u.disc.result = music.SearchPage{Tracks: u.tracks, Artists: []music.Named{{ID: "artist", Name: "Исполнитель"}}}
				u.rows = make([]widget.Clickable, u.exploreCount())
			case "new-update":
				u.up.available = &updates.Available{Manifest: updates.Manifest{Version: "99.0.0"}}
			case "updates":
				u.up.show = true
				u.up.list.Axis = layout.Vertical
				u.up.message = "Установлена последняя версия."
			case "actions":
				u.actionsMenu()
			case "settings":
				u.optionsMenu([]music.WaveOption{{Name: "Во время бега", Seed: "activity:running", Group: "context"}, {Name: "Треки: Открытия", Seed: "settingDiversity:discover", Group: "diversity"}})
			case "artists":
				u.page = "discovery"
				u.disc.result = music.SearchPage{Artists: []music.Named{{ID: "artist", Name: "Исполнитель"}}}
			}
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
			paint.Fill(gtx.Ops, bg)
			d := u.layout(gtx)
			if u.systemFonts {
				t.Fatal("ordinary UI loaded system font fallback", size, mode)
			}
			if d.Size.X > size.X || d.Size.Y > size.Y {
				t.Fatal("overflow", size, mode, d.Size)
			}
			if out != "" {
				w, e := headless.NewWindow(size.X, size.Y)
				if e != nil {
					t.Fatal(e)
				}
				if e = w.Frame(&ops); e != nil {
					w.Release()
					t.Fatal(e)
				}
				im := image.NewRGBA(image.Rectangle{Max: size})
				e = w.Screenshot(im)
				w.Release()
				if e != nil {
					t.Fatal(e)
				}
				if e = os.MkdirAll(out, 0700); e != nil {
					t.Fatal(e)
				}
				f, e := os.Create(filepath.Join(out, fmt.Sprintf("%dx%d-%s.png", size.X, size.Y, mode)))
				if e != nil {
					t.Fatal(e)
				}
				e = png.Encode(f, im)
				f.Close()
				if e != nil {
					t.Fatal(e)
				}
			}
		}
	}
}
