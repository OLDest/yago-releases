package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"yago/internal/audio"
	"yago/internal/music"
	"yago/internal/store"
	"yago/internal/zvuk"
)

func serviceTestUI(t *testing.T) *UI {
	t.Helper()
	old := stateDir
	stateDir = t.TempDir()
	t.Cleanup(func() { stateDir = old })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p := audio.New(t.TempDir(), nil)
	t.Cleanup(p.Close)
	return &UI{ctx: ctx, player: p, gen: 1, index: -1, theme: newUITheme(), services: servicesUI{enabled: true, mode: "yandex", yandex: music.New("y-token"), zvuk: &music.API{Backend: zvuk.New("z-token")}, yandexAccount: music.Account{UID: "11", DisplayName: "Яндекс"}, zvukAccount: music.Account{UID: "zvuk-22", DisplayName: "Звук"}}}
}
func TestServiceModesKeepAccountsAndIsolateLibrary(t *testing.T) {
	u := serviceTestUI(t)
	u.applyServiceMode(false)
	y, z := u.services.yandex, u.services.zvuk
	u.setTracks([]music.Track{{ID: "123"}})
	u.waveTracks = []music.Track{{ID: "123"}}
	u.session = "old-session"
	if !u.serviceAction("service-zvuk") || u.api != z || u.account.UID != "zvuk-22" || len(u.tracks) != 0 || len(u.waveTracks) != 0 || u.session != "" {
		t.Fatal("mode isolation")
	}
	if !u.serviceAction("service-all") {
		t.Fatal("all action")
	}
	if _, ok := u.api.Backend.(*music.Services); !ok {
		t.Fatal("no router")
	}
	if u.services.yandex != y || u.services.zvuk != z {
		t.Fatal("account lost on switch")
	}
	u.serviceAction("service-yandex")
	if u.api != y || u.account.UID != "11" {
		t.Fatal("yandex not restored")
	}
}
func TestAllSharesPerAccountBlocksWithoutIDCollisions(t *testing.T) {
	u := serviceTestUI(t)
	for _, p := range []store.Preferences{{Owner: "11", Tracks: map[music.ID]bool{"123": true}, Seeds: map[string]string{"context": "test"}}, {Owner: "zvuk-22", Tracks: map[music.ID]bool{"zvuk-123": true}}} {
		if e := store.SavePreferences(stateDir, p); e != nil {
			t.Fatal(e)
		}
	}
	u.services.mode = "all"
	u.applyServiceMode(false)
	if len(u.disc.prefs.Tracks) != 2 || u.disc.prefs.Seeds["context"] != "test" {
		t.Fatal(u.disc.prefs)
	}
	delete(u.disc.prefs.Tracks, "zvuk-123")
	if e := u.saveServicePreferences(u.disc.prefs); e != nil {
		t.Fatal(e)
	}
	y, _ := store.LoadPreferences(stateDir, "11")
	z, _ := store.LoadPreferences(stateDir, "zvuk-22")
	if !y.Tracks["123"] || len(z.Tracks) != 0 || y.Seeds["context"] != "test" {
		t.Fatal("cross-account preferences", y, z)
	}
}
func TestStaleServiceLoginCannotChangeAccount(t *testing.T) {
	u := serviceTestUI(t)
	u.services.authID = 4
	u.applyServiceMode(false)
	before := u.api
	if !u.serviceEvent(event{kind: "service-login", featureID: 3, token: music.Token{Access: "stale", Service: "zvuk"}, account: music.Account{UID: "zvuk-999"}}) || u.api != before {
		t.Fatal("stale login accepted")
	}
	u.serviceEvent(event{kind: "service-login", featureID: 4, err: errors.New("cancelled")})
	if u.api != before {
		t.Fatal("failed login replaced account")
	}
}

func TestServiceDropdownDismissalDoesNotClickUnderlyingRow(t *testing.T) {
	for _, inside := range []bool{true, false} {
		u := serviceTestUI(t)
		u.services.mode = "all"
		u.applyServiceMode(false)
		u.page = "tracks"
		u.setTracks([]music.Track{{ID: "123", Title: "Track"}})
		u.servicesMenu()
		var router input.Router
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1080, 740))}
		u.layout(gtx)
		router.Frame(&ops)
		x, y := float32(100), float32(280)
		if inside {
			x, y = 800, 70
		}
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, y)})
		clicked := u.disc.clicks[0].Clicked(gtx)
		dismiss := u.disc.dismiss.Clicked(gtx)
		if clicked != inside || dismiss == inside || u.rows[0].Clicked(gtx) {
			t.Fatal("service popup hit routing", inside, clicked, dismiss)
		}
	}
}

func TestProfilesAvailableWithoutAccount(t *testing.T) {
	u := serviceTestUI(t)
	u.services = servicesUI{}
	u.initServices()
	if !u.services.enabled || u.services.mode != "yandex" {
		t.Fatal("profiles unavailable without account")
	}
	u.serviceAction("service-zvuk")
	if u.api != nil || u.services.mode != "zvuk" {
		t.Fatal("anonymous API unexpectedly created")
	}
	u.servicesMenu()
	if len(u.disc.entries) != 4 || u.disc.entries[3].Action != "pick-wave-musify" {
		t.Fatal("profile choices missing")
	}
	u.serviceAction("service-musify")
	if u.api == nil || u.account.UID != "musify-guest" || u.waveLabel() != "Чарты" {
		t.Fatal("Musify guest mode unavailable")
	}
}
func TestSavedZvukRemainsAvailableAfterTemporaryStartupFailure(t *testing.T) {
	u := serviceTestUI(t)
	u.services.zvuk = nil
	u.services.zvukAccount = music.Account{}
	u.services.mode = "zvuk"
	u.services.authID = 7
	u.services.restoring = 2
	account := music.Account{UID: "zvuk-22", DisplayName: "Звук"}
	u.serviceEvent(event{kind: "service-restored", featureID: 7, token: music.Token{Service: "zvuk", Access: "saved", Profile: &account}, err: errors.New("temporary network failure")})
	u.serviceEvent(event{kind: "service-restored-empty", featureID: 7})
	if u.api == nil || u.account.UID != "zvuk-22" || u.authBusy || u.services.restoreWarning == "" {
		t.Fatal("saved profile discarded")
	}
}
func TestLocalHistoryRespectsAllAndSingleServiceModes(t *testing.T) {
	u := serviceTestUI(t)
	if e := store.RecordHistory(stateDir, "zvuk-22", music.Track{ID: "zvuk-track", Title: "Played"}, time.Now()); e != nil {
		t.Fatal(e)
	}
	u.services.mode = "all"
	if refs := u.readLocalHistory(); len(refs) != 1 || refs[0].ID != "zvuk-track" {
		t.Fatal("All omitted Zvuk history")
	}
	u.services.mode = "yandex"
	if len(u.readLocalHistory()) != 0 {
		t.Fatal("single-service history leaked")
	}
	u.services.mode = "zvuk"
	if len(u.readLocalHistory()) != 1 {
		t.Fatal("Zvuk history missing")
	}
}
func TestServicesLayoutFitsWithoutExtraFonts(t *testing.T) {
	u := serviceTestUI(t)
	u.applyServiceMode(false)
	for _, size := range []image.Point{image.Pt(380, 820), image.Pt(800, 480), image.Pt(1080, 740), image.Pt(1600, 829)} {
		for _, mode := range []string{"home", "services", "mixed", "waves", "settings-yandex", "settings-zvuk", "choice-yandex", "choice-zvuk", "settings-all", "choice-all-yandex", "choice-all-zvuk", "choice-all-shared", "choice-all-genre", "choice-all-language"} {
			u.services.mode = "all"
			u.applyServiceMode(false)
			u.closeMenu()
			u.page = "home"
			u.services.mode = "all"
			switch mode {
			case "settings-all", "choice-all-yandex", "choice-all-zvuk", "choice-all-shared", "choice-all-genre", "choice-all-language":
				options := []music.WaveOption{}
				for _, o := range []music.WaveOption{{Name: "Моя волна", Seed: "user:onyourwave", Group: "context"}, {Name: "Танцую", Seed: "activity:dance", Group: "context"}, {Name: "Просыпаюсь", Seed: "activity:wake", Group: "context"}, {Name: "В дороге", Seed: "activity:road", Group: "context"}, {Name: "Работаю", Seed: "activity:work", Group: "context"}, {Name: "Тренируюсь", Seed: "activity:run", Group: "context"}, {Name: "Отдыхаю", Seed: "activity:rest", Group: "context"}, {Name: "Засыпаю", Seed: "activity:sleep", Group: "context"}, {Name: "Подбор: Любое", Seed: "diversity:any", Group: "diversity"}, {Name: "Под настроение: Любое", Seed: "settingMoodEnergy:all", Group: "moodEnergy"}, {Name: "По языку: Любой", Seed: "settingLanguage:any", Group: "language"}} {
					o.Group = "yandex/" + o.Group
					options = append(options, o)
				}
				zs, e := u.services.zvuk.WaveOptions(u.ctx)
				if e != nil {
					t.Fatal(e)
				}
				for _, o := range zs {
					o.Group = "zvuk/" + o.Group
					options = append(options, o)
				}
				for _, c := range sharedWaveChoices {
					if c.YGroup == "moodEnergy" || c.YGroup == "genre" || c.YGroup == "language" {
						options = append(options, music.WaveOption{Name: c.Name, Group: "yandex/" + c.YGroup, Seed: c.YSeed})
					}
				}
				u.optionsMenu(options)
				if mode == "choice-all-language" {
					u.waveGroupMenu("shared/language")
				}
				if mode == "choice-all-genre" {
					u.waveGroupMenu("shared/genre")
				}
				if mode == "choice-all-shared" {
					u.waveGroupMenu("shared/mood")
				}
				if mode == "choice-all-yandex" {
					u.waveGroupMenu("yandex/context")
				}
				if mode == "choice-all-zvuk" {
					u.waveGroupMenu("zvuk/popular")
				}
			case "settings-yandex", "choice-yandex":
				u.services.mode = "yandex"
				u.services.waveSource = "yandex"
				u.optionsMenu([]music.WaveOption{{Name: "Моя волна", Seed: "user:onyourwave", Group: "context"}, {Name: "Танцую", Seed: "activity:dance", Group: "context"}, {Name: "Просыпаюсь", Seed: "activity:wake", Group: "context"}, {Name: "По характеру: Любимое", Seed: "diversity:favorite", Group: "diversity"}, {Name: "По характеру: Любое", Seed: "diversity:any", Group: "diversity"}, {Name: "Под настроение: Спокойное", Seed: "settingMoodEnergy:calm", Group: "moodEnergy"}, {Name: "Под настроение: Любое", Seed: "settingMoodEnergy:all", Group: "moodEnergy"}, {Name: "По языку: Русский", Seed: "settingLanguage:russian", Group: "language"}, {Name: "По языку: Любой", Seed: "settingLanguage:any", Group: "language"}})
				if mode == "choice-yandex" {
					u.waveGroupMenu("context")
				}
			case "settings-zvuk", "choice-zvuk":
				u.services.mode = "zvuk"
				u.services.waveSource = "zvuk"
				options, e := u.services.zvuk.WaveOptions(u.ctx)
				if e != nil {
					t.Fatal(e)
				}
				u.optionsMenu(options)
				if mode == "choice-zvuk" {
					u.waveGroupMenu("energy")
				}
			case "waves":
				u.waveTracks = []music.Track{{ID: "123"}, {ID: "zvuk-123"}}
				u.services.waveSource = "zvuk"
				u.chooseWave()
			case "services":
				u.servicesMenu()
			case "mixed":
				u.page = "tracks"
				u.setTracks([]music.Track{{ID: "123", Title: "Яндекс"}, {ID: "zvuk-123", Title: "Звук"}})
			}
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
			paint.Fill(&ops, bg)
			d := u.layout(gtx)
			if d.Size.X > size.X || d.Size.Y > size.Y {
				t.Fatal(size, mode, d.Size)
			}
			if u.systemFonts {
				t.Fatal("extra fonts", mode)
			}
			if out := os.Getenv("YAGO_SERVICES_RENDER_DIR"); out != "" {
				w, e := headless.NewWindow(size.X, size.Y)
				if e != nil {
					t.Fatal(e)
				}
				e = w.Frame(&ops)
				if e != nil {
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
