package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"yago/internal/audio"
	"yago/internal/metrics"
	"yago/internal/mobile"
	"yago/internal/music"
	"yago/internal/store"
	"yago/internal/updates"
)

var updateTicket string
var stateDir string
var demo bool
var demoURL string
var windowMode string

// main starts YaGo and sends local playback diagnostics to the selected state directory.
func main() {
	if updates.RunInstaller() {
		return
	}
	exe, _ := os.Executable()
	defaultDir := filepath.Join(filepath.Dir(exe), "state")
	if runtime.GOOS == "android" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		defaultDir = filepath.Join(base, "yago")
	}
	flag.StringVar(&stateDir, "state", defaultDir, "Local state directory")
	flag.BoolVar(&demo, "demo", false, "Show sample library without signing in")
	flag.StringVar(&demoURL, "demo-audio", "", "MP3 URL for local playback verification")
	flag.StringVar(&updateTicket, "update-ticket", "", "Update health marker")
	flag.Parse()
	debug.SetMemoryLimit(96 << 20)
	debug.SetGCPercent(75)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		log.Fatal(err)
	}
	f, e := os.OpenFile(filepath.Join(stateDir, "app.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e == nil {
		defer f.Close()
		log.SetOutput(f)
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("YaGo startup version=%s runtime=%s", updates.Version, runtime.Version())
	log.Print("network profile: tls-client Chrome 154; HTTP/1.1 and HTTP/2; certificate verification enabled")
	go func() {
		w := new(app.Window)
		if runtime.GOOS == "android" {
			w.Option(app.Title("YaGo"), app.StatusColor(bg), app.NavigationColor(bg))
		} else if windowMode == "phone" {
			w.Option(app.Title("YaGo · Phone preview"), app.Size(unit.Dp(380), unit.Dp(820)), app.MinSize(unit.Dp(320), unit.Dp(480)))
		} else {
			w.Option(app.Title("YaGo · Музыка"), app.Size(unit.Dp(1080), unit.Dp(740)), app.MinSize(unit.Dp(320), unit.Dp(480)))
		}
		if e := run(w); e != nil {
			log.Print(e)
		}
		os.Exit(0)
	}()
	app.Main()
}

type event struct {
	chartEntries    []menuEntry
	feedbackTrack   music.ID
	feedbackLike    *bool
	waveContexts    []music.WaveContext
	featureID       uint64
	explore         *exploreEvent
	waveID          uint64
	qualitySequence uint64
	quality         music.Quality
	streamInfo      music.StreamInfo
	qualities       []music.Quality
	codec           string
	qualityFallback bool
	streamID        uint64
	bitrate         int
	kind            string
	gen             uint64
	err             error
	tracks          []music.Track
	playlists       []music.Playlist
	account         music.Account
	token           music.Token
	code            music.DeviceCode
	session, batch  string
	message         string
}
type UI struct {
	chartAdvanced                                                                                                    bool
	chartDraft                                                                                                       string
	musifyLists                                                                                                      musifyListsUI
	musifyLogin                                                                                                      musifyLoginUI
	trackFeedback                                                                                                    trackFeedbackUI
	historyPlayID                                                                                                    uint64
	services                                                                                                         servicesUI
	waveContexts, waveViewContexts                                                                                   []music.WaveContext
	waveWaiting                                                                                                      bool
	waveRetryAt                                                                                                      time.Time
	fit                                                                                                              fitUI
	disc                                                                                                             discoveryUI
	waveRadioStart                                                                                                   bool
	waveRefresh                                                                                                      widget.Clickable
	waveRefreshCancel                                                                                                context.CancelFunc
	waveRefreshID                                                                                                    uint64
	waveRefreshing                                                                                                   bool
	waveViewSession, waveViewBatch                                                                                   string
	waveViewRefreshed                                                                                                bool
	waveNextCancel                                                                                                   context.CancelFunc
	waveNextID                                                                                                       uint64
	viewWave                                                                                                         bool
	waveTracks                                                                                                       []music.Track
	off                                                                                                              offlineUI
	queueOffline                                                                                                     bool
	offlineQueuePlaylist                                                                                             string
	libraryCancel                                                                                                    context.CancelFunc
	qualityMenuOpen, qualitySwitching                                                                                bool
	qualityFLAC                                                                                                      widget.Clickable
	qualityBasic, qualityHQ, qualityDismiss                                                                          widget.Clickable
	qualityCancel                                                                                                    context.CancelFunc
	qualitySequence                                                                                                  uint64
	pendingQuality                                                                                                   music.Quality
	checkpointSuspended                                                                                              bool
	reloadingBest, restoringPlayback                                                                                 bool
	pendingPlayback                                                                                                  *store.Playback
	pauseRequested                                                                                                   bool
	requestedOffset                                                                                                  float64
	lastCheckpoint                                                                                                   time.Time
	lastCheckpointKey                                                                                                checkpointKey
	hasCheckpoint, checkpointError                                                                                   bool
	qualities                                                                                                        []music.Quality
	stopped                                                                                                          bool
	quality                                                                                                          music.Quality
	qualityClick, mute                                                                                               widget.Clickable
	muted                                                                                                            bool
	streamCodec                                                                                                      string
	streamBitrate                                                                                                    int
	streamVersion                                                                                                    uint64
	shuffle                                                                                                          widget.Clickable
	shuffled                                                                                                         bool
	order                                                                                                            []int
	orderPos                                                                                                         int
	up                                                                                                               updateUI
	mu                                                                                                               sync.Mutex
	playID                                                                                                           uint64
	w                                                                                                                *app.Window
	theme                                                                                                            *material.Theme
	systemFonts                                                                                                      bool
	section                                                                                                          librarySection
	listSwipe                                                                                                        listSwipe
	events                                                                                                           chan event
	ctx                                                                                                              context.Context
	cancel                                                                                                           context.CancelFunc
	api                                                                                                              *music.API
	account                                                                                                          music.Account
	device                                                                                                           string
	player                                                                                                           *audio.Player
	login, open, logout, search, wave, likes, lists, back, next, pause, previous, like, minus, plus, rewind, forward widget.Clickable
	editor                                                                                                           widget.Editor
	list                                                                                                             widget.List
	rows                                                                                                             []widget.Clickable
	tracks                                                                                                           []music.Track
	playlists                                                                                                        []music.Playlist
	queue                                                                                                            []music.Track
	index                                                                                                            int
	current                                                                                                          music.Track
	page, title, status, code                                                                                        string
	busy, authBusy                                                                                                   bool
	gen                                                                                                              uint64
	loginCancel                                                                                                      context.CancelFunc
	volume                                                                                                           float32
	session, batch                                                                                                   string
	isWave                                                                                                           bool
	advancePending                                                                                                   bool
	failedTracks                                                                                                     int
	serviceActive                                                                                                    bool
	pageScroll                                                                                                       widget.List
}

var bg = color.NRGBA{R: 17, G: 19, B: 24, A: 255}
var panel = color.NRGBA{R: 25, G: 28, B: 35, A: 255}
var muted = color.NRGBA{R: 151, G: 159, B: 176, A: 255}
var accent = color.NRGBA{R: 250, G: 203, B: 74, A: 255}
var white = color.NRGBA{R: 238, G: 241, B: 247, A: 255}

func run(w *app.Window) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &UI{w: w, ctx: ctx, cancel: cancel, theme: newUITheme(), events: make(chan event, 64), title: "Ваша музыка. Без лишнего.", page: "home", volume: .65, index: -1, gen: 1}
	u.editor.SingleLine = true
	u.editor.Submit = true
	u.list.Axis = layout.Vertical
	u.pageScroll.Axis = layout.Vertical
	u.player = audio.New(filepath.Join(stateDir, "audio"), w.Invalidate)
	defer u.player.Close()
	u.initOffline()
	if u.off.manager != nil {
		defer u.off.manager.Close()
	}
	if runtime.GOOS != "android" || os.Getenv("YAGO_METRICS") == "1" {
		go func() {
			timer := time.NewTicker(5 * time.Second)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					metrics.Write(filepath.Join(stateDir, "metrics.json"))
				}
			}
		}()
	}
	var err error
	u.device, err = store.Device(stateDir)
	if err != nil {
		u.status = "Не удалось сохранить настройки"
	}
	u.initServices()
	if demo {
		u.account.DisplayName = "Демо"
		u.title = "Демонстрационная библиотека"
		u.page = "tracks"
		u.setTracks(demoTracks())
		u.status = "Демо · для Яндекс Музыки войдите в аккаунт"
	} else if !u.loadServiceCredentials() {
		go func() {
			t, e := store.Load(u.device)
			if e == nil {
				u.emit(event{kind: "stored", token: t, gen: 1})
			}
		}()
	}
	if b, e := os.ReadFile(filepath.Join(stateDir, "shuffle")); e == nil && string(b) == "1" {
		u.shuffled = true
	}
	u.initPlaybackSettings()
	u.initPlaybackRestore()
	u.initPlatform()
	u.initUpdates()
	var ops op.Ops
	startupFrames := 8
	tick := time.NewTimer(time.Second)
	defer tick.Stop()
	startupTicks := 0
	controllerDone := make(chan struct{})
	defer func() {
		cancel()
		<-controllerDone
		u.mu.Lock()
		u.savePlayback(true)
		u.mu.Unlock()
		_ = mobile.Publish(mobile.Playback{})
	}()
	go func() {
		defer close(controllerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case command := <-mobile.Commands:
				u.mu.Lock()
				u.mediaCommand(command)
				u.mu.Unlock()
				w.Invalidate()
				tick.Reset(0)
			case <-u.player.Changes():
				tick.Reset(0)
			case f := <-u.up.events:
				u.mu.Lock()
				f()
				u.mu.Unlock()
				w.Invalidate()
				tick.Reset(0)
			case ev := <-u.events:
				u.mu.Lock()
				u.handle(ev)
				u.mu.Unlock()
				w.Invalidate()
				tick.Reset(0)
			case <-tick.C:
				u.mu.Lock()
				u.backgroundTick()
				u.tickUpdates()
				interval := u.backgroundInterval()
				u.mu.Unlock()
				if startupTicks < 10 {
					startupTicks++
					w.Invalidate()
					interval = time.Second
				}
				tick.Reset(interval)
			}
		}
	}()
	for {
		ev := w.Event()
		u.platformEvent(ev)
		switch e := ev.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			u.mu.Lock()
			gtx := app.NewContext(&ops, e)
			u.update(gtx)
			paint.Fill(gtx.Ops, bg)
			u.layout(gtx)
			if startupFrames > 0 {
				startupFrames--
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(100 * time.Millisecond)})
			}
			u.mu.Unlock()
			e.Frame(gtx.Ops)
			if updateTicket != "" {
				_ = os.WriteFile(updateTicket, []byte(updates.Version), 0600)
				updateTicket = ""
			}
		}
	}
}
func demoTracks() []music.Track {
	return []music.Track{{ID: "demo-1", Title: "Проверка воспроизведения", Artists: []music.Named{{Name: "Локальный тест"}}, DurationMS: 240000}, {ID: "demo-2", Title: "Поиск, коллекция и очередь", Artists: []music.Named{{Name: "YaGo"}}, DurationMS: 198000}, {ID: "demo-3", Title: "Войдите для доступа к своей музыке", Artists: []music.Named{{Name: "Яндекс Музыка"}}, DurationMS: 184000}}
}
func (u *UI) emit(e event) {
	select {
	case u.events <- e:
		if u.w != nil {
			u.w.Invalidate()
		}
	case <-u.ctx.Done():
	}
}
func (u *UI) setTracks(tracks []music.Track) {
	u.fit.offset = 0
	u.fit.followID = ""
	u.tracks = tracks
	u.playlists = nil
	u.rows = make([]widget.Clickable, len(tracks))
	u.off.actions = make([]widget.Clickable, len(tracks))
	u.trackFeedback.likes = make([]widget.Clickable, len(tracks))
	u.trackFeedback.blocks = make([]widget.Clickable, len(tracks))
	u.list.Position = layout.Position{}
}
func (u *UI) begin(title string) (context.Context, context.CancelFunc, uint64) {
	u.cancelLibrary()
	u.gen++
	u.title = title
	u.busy = true
	u.status = "Загружаем…"
	ctx, cancel := context.WithCancel(u.ctx)
	u.libraryCancel = cancel
	return ctx, cancel, u.gen
}
func (u *UI) handle(e event) {
	if e.kind == "musify-chart-group" {
		if e.gen != u.gen || u.disc.menu != "loading" {
			return
		}
		if e.err != nil {
			u.openChartSettings()
			u.status = e.err.Error()
		} else {
			u.showMenu("musify-chart-settings", e.chartEntries)
		}
		return
	}
	if e.kind == "musify-playlist-result" {
		u.musifyLists.busy = false
		if e.err != nil {
			u.status = e.err.Error()
		} else {
			u.status = "Плейлист обновлён"
			u.musifyLists.name.SetText("")
			u.readMusifyLists()
		}
		return
	}
	if e.kind == "musify-playlists-read" {
		f := &u.musifyLists
		f.busy = false
		if e.err != nil {
			u.status = e.err.Error()
			return
		}
		f.lists = e.playlists
		f.rows = make([]widget.Clickable, len(f.lists))
		found := false
		for _, p := range f.lists {
			if p.Kind == f.selected {
				found = true
				break
			}
		}
		if !found {
			f.selected = ""
		}
		return
	}
	if e.kind == "musify-login-result" {
		u.musifyLogin.busy = false
		if e.err != nil {
			u.status = "Вход Musify: " + e.err.Error()
		} else {
			u.musifyLogin.open = false
			if u.services.mode == "all" {
				u.applyServiceMode(false)
			}
			u.status = "Вход Musify выполнен"
		}
		return
	}
	if u.serviceEvent(e) {
		return
	}
	if u.featureEvent(e) {
		return
	}
	if e.kind == "wave-refresh" {
		u.waveRefreshReady(e)
		return
	}
	if e.kind == "wave-next" || e.kind == "wave-settings-tail" {
		u.waveContinuationReady(e)
		return
	}
	if e.kind == "quality-ready" {
		u.qualityReady(e)
		return
	}
	if e.kind == "stream" {
		if e.streamID == u.streamVersion {
			u.streamBitrate = e.bitrate
			u.streamCodec = e.codec
			u.qualities = append([]music.Quality(nil), e.qualities...)
			if e.qualityFallback {
				u.quality = music.High
				_ = os.WriteFile(filepath.Join(stateDir, "quality"), []byte("320"), 0600)
				u.status = fmt.Sprintf("Выбранный битрейт недоступен. Авто: MP3 %d кбит/с", e.bitrate)
			}
		}
		return
	}
	if e.gen != 0 && e.gen != u.gen {
		return
	}
	switch e.kind {
	case "stored":
		u.authBusy = true
		u.status = "Восстанавливаем вход…"
		u.checkToken(e.token, true)
	case "code":
		u.code = e.code.User
		u.status = "Откройте ya.ru/device и введите код"
		u.authBusy = true
	case "token":
		u.code = ""
		u.checkToken(e.token, false)
	case "account":
		u.authBusy = false
		if e.err != nil {
			if u.queueOffline {
				u.status = "Локальная музыка доступна без сети"
			} else {
				u.status = e.err.Error()
			}
			break
		}
		u.api = music.New(e.token.Access)

		u.account = e.account
		u.initPreferences()
		if !u.isOfflineView() && !u.queueOffline {
			u.page = "home"
			u.title = "Музыка для вас"
			u.setTracks(nil)
		}
		u.status = "Вы вошли. Выберите музыку."
		u.code = ""
		if err := store.Save(u.device, e.token); err != nil {
			u.status = "Вход выполнен. Системное хранилище недоступно: вход сохранён только до закрытия."
		}
		if !u.queueOffline {
			u.restorePlayback()
		}
	case "auth-error":
		u.authBusy = false
		u.code = ""
		if e.err != nil {
			u.status = e.err.Error()
		}
	case "tracks":
		u.busy = false
		if e.err != nil {
			u.status = e.err.Error()
			break
		}
		u.page = "tracks"
		u.viewWave = false
		if u.services.mode == "musify" && u.section == sectionWave {
			e.tracks = u.filterRecommendations(e.tracks)
		}
		u.setTracks(e.tracks)
		u.status = fmt.Sprintf("Треков: %d", len(e.tracks))
		if len(e.tracks) == 0 {
			u.status = "Ничего не найдено"
		}
	case "playlists":
		u.viewWave = false
		u.busy = false
		if e.err != nil && !music.IsPartialLibrary(e.err) {
			u.status = e.err.Error()
			break
		}
		u.page = "playlists"
		u.tracks = nil
		u.playlists = e.playlists
		u.rows = make([]widget.Clickable, len(e.playlists))
		u.list.Position = layout.Position{}
		u.status = fmt.Sprintf("Плейлистов: %d", len(e.playlists))
		if e.err != nil {
			u.status += " · " + e.err.Error()
		}
	case "wave":
		u.busy = false
		u.advancePending = false
		if e.err != nil {
			u.status = e.err.Error()
			break
		}
		e.tracks = u.filterRecommendations(e.tracks)
		u.cancelWaveContinuation()
		u.waveContexts = waveContexts(len(e.tracks), e.session, e.batch)
		u.waveViewContexts = u.waveContexts
		u.queueOffline = false
		u.offlineQueuePlaylist = ""
		u.waveTracks = append([]music.Track(nil), e.tracks...)
		u.waveViewSession, u.waveViewBatch = e.session, e.batch
		u.waveViewRefreshed = false
		u.viewWave = true
		u.isWave = true
		u.session = e.session
		u.batch = e.batch
		u.page = "tracks"
		u.setTracks(e.tracks)
		u.queue = append([]music.Track(nil), e.tracks...)
		u.index = -1
		u.resetOrder(0)
		u.status = u.waveTitle()
		if len(u.queue) > 0 {
			u.start(0)
			if u.waveMixed() {
				u.startWaveRefill()
			}
		} else {
			u.status = "Волна не вернула треки"
		}
	case "message":
		if e.err != nil {
			u.status = e.err.Error()
		} else {
			u.status = e.message
		}
	}
}
func (u *UI) checkToken(t music.Token, refresh bool) {
	ctx := u.ctx
	g := u.gen
	go func() {
		a := music.New(t.Access)
		account, err := a.Account(ctx)
		if err != nil && refresh && t.Refresh != "" {
			fresh, e := music.Refresh(ctx, t.Refresh)
			if e == nil {
				if fresh.Refresh == "" {
					fresh.Refresh = t.Refresh
				}
				t = fresh
				a = music.New(t.Access)
				account, err = a.Account(ctx)
			}
		}
		u.emit(event{kind: "account", account: account, token: t, err: err, gen: g})
	}()
}
func (u *UI) startLogin() {
	g := u.gen
	ctx, cancel := u.prepareProfileLogin()
	u.status = "Получаем код входа…"
	id := u.device
	go func() {
		defer cancel()
		code, err := music.StartLogin(ctx, id)
		if err != nil {
			u.emit(event{kind: "auth-error", err: err, gen: g})
			return
		}
		u.emit(event{kind: "code", code: code, gen: g})
		interval := time.Duration(max(1, code.Interval)) * time.Second
		deadline := time.NewTimer(time.Duration(max(1, code.Expires)) * time.Second)
		defer deadline.Stop()
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-deadline.C:
				u.emit(event{kind: "auth-error", err: fmt.Errorf("код истёк. Нажмите «Войти» ещё раз"), gen: g})
				return
			case <-timer.C:
				t, e := music.PollLogin(ctx, code.Device)
				if ctx.Err() != nil {
					return
				}
				if e == nil && t.Access != "" {
					u.emit(event{kind: "token", token: t, gen: g})
					return
				}
				if e == nil && t.Error != "authorization_pending" && t.Error != "slow_down" {
					u.emit(event{kind: "auth-error", err: fmt.Errorf("вход отклонён. Попробуйте ещё раз"), gen: g})
					return
				}
				if t.Error == "slow_down" {
					interval += 5 * time.Second
				}
				timer.Reset(interval)
			}
		}
	}()
}
func (u *UI) load(kind string) {
	if kind == "likes" {
		u.section = sectionLikes
		u.loadExplore(exploreView{Kind: "likes", Title: "Мне нравится"})
		return
	}
	if kind == "search" {
		u.searchExplore()
		return
	}

	a := u.api
	if kind == "playlists" {
		a = u.playlistLibraryAPI()
	}
	if a == nil {
		u.status = "Сначала войдите в аккаунт"
		return
	}
	uid := u.account.UID
	if kind == "likes" {
		u.section = sectionLikes
		u.viewWave = false
	} else if kind == "playlists" {
		u.section = sectionPlaylists
		u.viewWave = false
	}
	title := map[string]string{"likes": "Мне нравится", "playlists": "Мои плейлисты", "search": "Результаты поиска"}[kind]
	q := strings.TrimSpace(u.editor.Text())
	if kind == "search" && q == "" {
		return
	}
	ctx, cancel, g := u.begin(title)
	go func() {
		defer cancel()
		switch kind {
		case "likes":
			t, e := a.Likes(ctx, uid)
			u.emit(event{kind: "tracks", gen: g, tracks: t, err: e})
		case "search":
			t, e := a.Search(ctx, q)
			u.emit(event{kind: "tracks", gen: g, tracks: t, err: e})
		case "playlists":
			p, e := a.Playlists(ctx, uid)
			u.emit(event{kind: "playlists", gen: g, playlists: p, err: e})
		}
	}()
}
func (u *UI) loadPlaylist(p music.Playlist) {
	a, uid := u.playlistLibraryAPI(), p.Owner.UID
	if a == nil {
		u.status = "Подключите аккаунт для плейлистов"
		return
	}
	if uid == "" {
		uid = u.account.UID
	}
	ctx, cancel, g := u.begin(p.Title)
	u.disc.view = exploreView{Kind: "playlist", ID: p.Kind, Title: p.Title}
	go func() {
		defer cancel()
		t, e := a.Playlist(ctx, uid, p.Kind)
		u.emit(event{kind: "tracks", gen: g, tracks: t, err: e})
	}()
}
func (u *UI) loadWave(fresh bool) {
	if !fresh {
		u.continueWave()
		return
	}
	if u.api == nil {
		u.status = "Сначала войдите в аккаунт"
		return
	}
	a := u.waveAPI()
	session := u.session
	if fresh {
		session = ""
	}
	var queue []string
	for _, t := range u.queue[max(0, len(u.queue)-10):] {
		queue = append(queue, string(t.ID))
	}
	ctx, cancel, g := u.begin(u.waveTitle())
	seeds := u.waveSeeds()
	u.advancePending = true
	go func() {
		defer cancel()
		v, e := a.WaveWithSeeds(ctx, session, queue, seeds)
		if e != nil && session != "" && ctx.Err() == nil {
			v, e = a.WaveWithSeeds(ctx, "", queue, seeds)
		}
		tracks := make([]music.Track, 0, len(v.Sequence))
		for _, t := range v.Sequence {
			tracks = append(tracks, t.Track)
		}
		if e == nil && fresh {
			_ = a.Feedback(ctx, v.Session, v.Batch, "radioStarted", music.Track{}, 0)
		}
		u.emit(event{kind: "wave", gen: g, tracks: tracks, session: v.Session, batch: v.Batch, err: e})
	}()
}
func (u *UI) feedback(kind string) {
	if u.queueOffline {
		return
	}
	if !u.isWave || u.api == nil || u.session == "" || u.current.ID == "" {
		return
	}
	a, ctx, session, batch, t, pos := u.api, u.ctx, u.session, u.batch, u.current, u.player.Snapshot().Position
	if kind == "trackStarted" {
		pos = 0
	}
	radioStart := kind == "trackStarted" && u.waveRadioStart
	if radioStart {
		u.waveRadioStart = false
	}
	go func() {
		if radioStart {
			_ = a.Feedback(ctx, session, batch, "radioStarted", music.Track{}, 0)
		}
		if e := a.Feedback(ctx, session, batch, kind, t, pos); e != nil {
			u.emit(event{kind: "message", message: "Не удалось отправить отклик волны"})
		}
	}()
}
func (u *UI) start(i int) {
	u.failedTracks = 0
	u.startAt(i, 0, false, true)
}
func (u *UI) startAt(i int, offset float64, paused, notifyStart bool) {
	u.startPrepared(i, offset, paused, notifyStart, nil)
}

// startPrepared selects a track, resolves its stream and starts or restores playback.
func (u *UI) startPrepared(i int, offset float64, paused, notifyStart bool, prepared *music.StreamInfo) {
	if !u.queueOffline && !demo && (i < 0 || i >= len(u.queue) || u.trackPlaybackAPI(u.queue[i].ID) == nil) {
		u.status = "Сначала войдите в аккаунт"
		return
	}
	if i < 0 || i >= len(u.queue) {
		return
	}
	if u.blocked(u.queue[i]) {
		u.status = "Трек или исполнитель заблокирован. Ещё → Заблокированные."
		return
	}
	if !u.isWave {
		u.cancelWaveContinuation()
		u.waveContexts = nil
		u.session, u.batch = "", ""
	} else if len(u.waveContexts) == len(u.queue) {
		c := u.waveContexts[i]
		if c.Session != "" {
			if c.Session != u.session {
				u.waveRadioStart = true
			}
			u.session, u.batch = c.Session, c.Batch
		}
	}
	u.index = i
	for p, v := range u.order {
		if v == i {
			u.orderPos = p
			break
		}
	}
	u.cancelQualityChange()
	u.checkpointSuspended = false
	u.reloadingBest = false
	u.restoringPlayback = false
	u.stopped = false
	u.pauseRequested = paused
	offset = max(0, offset)
	if duration := float64(u.queue[i].DurationMS) / 1000; duration > 0 {
		offset = min(offset, max(0, duration-.25))
	}
	u.requestedOffset = offset
	u.current = u.queue[i]

	u.streamBitrate = 0
	u.streamCodec = ""
	u.qualities = nil
	u.streamVersion++
	u.advancePending = false
	t := u.current
	u.pendingPlayback = nil
	if u.queueOffline {
		if u.off.library == nil {
			u.status = "Оффлайн-хранилище недоступно"
			u.playID = u.player.PlayAt(func(context.Context) (string, error) {
				return "", errors.New("Оффлайн-хранилище недоступно")
			}, offset, paused)
			return
		}
		path, saved, e := u.off.library.Resolve(t.ID)
		if e != nil {
			u.status = "Локальный файл недоступен. Скачайте трек заново."
			u.playID = u.player.PlayAt(func(context.Context) (string, error) {
				return "", errors.New("Локальный файл недоступен. Скачайте трек заново.")
			}, offset, paused)
			return
		}
		demo = false
		u.isWave = false
		u.streamBitrate = saved.Bitrate
		u.streamCodec = saved.Codec
		u.serviceActive = true
		if runtime.GOOS == "android" {
			_ = mobile.Publish(mobile.Playback{Title: t.Title, Artist: t.Artist(), Duration: t.DurationMS, Active: true, Loading: !paused})
		}
		u.playID = u.player.PlayVerifiedFileAt(path, saved.SHA256, offset, paused)
		u.savePlayback(true)
		return
	}
	if demo {
		if demoURL == "" {
			u.status = "Для прослушивания войдите в аккаунт"
			return
		}
		u.playID = u.player.PlayAt(func(context.Context) (string, error) { return demoURL, nil }, offset, paused)
		u.savePlayback(true)
		return
	}
	a := u.trackPlaybackAPI(t.ID)
	u.serviceActive = true
	if runtime.GOOS == "android" {
		_ = mobile.Publish(mobile.Playback{Title: t.Title, Artist: t.Artist(), Duration: t.DurationMS, Active: true, Loading: !paused})
	}
	quality, version := u.quality, u.streamVersion
	u.playID = u.player.PlayAt(func(ctx context.Context) (string, error) {
		log.Printf("audio resolve track_id=%s quality=%d prepared=%t", t.ID, quality, prepared != nil)
		var info music.StreamInfo
		var err error
		if prepared != nil {
			info = *prepared
		} else {
			info, err = a.Stream(ctx, t, quality)
		}
		if err == nil {
			log.Printf("audio resolved track_id=%s codec=%s bitrate=%d", t.ID, info.Codec, info.Bitrate)
			u.emit(event{kind: "stream", streamID: version, codec: info.Codec, bitrate: info.Bitrate, qualities: info.Available, qualityFallback: info.Fallback})
		} else {
			log.Printf("audio resolve failed track_id=%s error_type=%T", t.ID, err)
		}
		return info.URL, err
	}, offset, paused)
	if notifyStart {
		u.feedback("trackStarted")
	}
	u.savePlayback(true)
	u.maybePrefetchWave()
}
func (u *UI) advance(auto bool) {
	if u.isWave && u.waveWaiting && u.waveNextCancel != nil {
		return
	}
	if len(u.queue) == 0 {
		return
	}
	if auto {
		u.feedback("trackFinished")
	} else {
		u.feedback("skip")
	}
	if next := u.nextIndex(); next >= 0 {
		u.start(next)
	} else if u.isWave {
		u.loadWave(false)
	} else {
		u.status = "Очередь закончилась"
		u.advancePending = true
	}
}
func (u *UI) update(gtx layout.Context) {
	if direction := u.listSwipe.update(gtx, u.swipeEnabled() && !gtx.Focused(&u.off.editor)); direction != 0 {
		u.turnListPage(direction)
	}
	u.refreshShortcut(gtx)
	u.fitInputs(gtx)
	u.discoveryInputs(gtx)
	for u.waveRefresh.Clicked(gtx) {
		u.refreshWave()
	}
	u.trackFeedbackInputs(gtx)
	u.offlineInputs(gtx)
	u.updateInputs(gtx)
	for u.qualityClick.Clicked(gtx) {
		if u.disc.menu == "actions" {
			u.closeMenu()
		}
		u.qualityMenuOpen = !u.qualityMenuOpen
	}
	for u.qualityDismiss.Clicked(gtx) {
		u.qualityMenuOpen = false
	}
	for u.qualityBasic.Clicked(gtx) {
		u.selectQuality(music.Normal)
	}
	for u.qualityFLAC.Clicked(gtx) {
		u.selectQuality(music.Lossless)
	}
	for u.qualityHQ.Clicked(gtx) {
		u.selectQuality(music.High)
	}
	for u.mute.Clicked(gtx) {
		u.muteVolume()
	}
	for u.shuffle.Clicked(gtx) {
		u.toggleShuffle()
	}

	for u.login.Clicked(gtx) {
		demo = false
		u.chooseLogin()
	}
	for u.open.Clicked(gtx) {
		openBrowser("https://ya.ru/device")
	}
	for u.logout.Clicked(gtx) {
		if u.manageServices() {
			continue
		}
		u.cancelWaveRefresh()
		u.waveViewSession = ""
		u.waveViewBatch = ""
		u.waveViewRefreshed = false
		u.waveTracks = nil
		u.waveContexts = nil
		u.waveViewContexts = nil
		u.session = ""
		u.batch = ""
		if u.off.manager != nil {
			u.off.manager.Cancel()
		}
		u.queueOffline = false
		u.cancelLibrary()
		if u.loginCancel != nil {
			u.loginCancel()
		}
		u.gen++
		demo = false
		u.player.Stop()
		u.serviceActive = false
		u.api = nil

		u.account = music.Account{}
		u.disc.prefs = store.Preferences{}
		u.disc.pending = false
		u.disc.pendingID++
		u.cancelQualityChange()
		u.qualityMenuOpen = false
		u.current = music.Track{}
		u.queue = nil
		u.releaseLibraryView()
		u.isWave = false
		u.authBusy = false
		u.code = ""
		u.page = "home"
		u.title = "Ваша музыка. Без лишнего."
		u.status = "Вы вышли"
		u.clearPlayback()
		go func() {
			if e := store.Delete(u.device); e != nil {
				u.emit(event{kind: "message", err: fmt.Errorf("не удалось удалить сохранённый вход")})
			}
		}()
	}
	for u.search.Clicked(gtx) {
		u.load("search")
	}
	for {
		e, ok := u.editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := e.(widget.SubmitEvent); ok {
			u.load("search")
		}
	}
	for u.likes.Clicked(gtx) {
		u.load("likes")
	}
	for u.lists.Clicked(gtx) {
		u.closeMenu()
		u.load("playlists")
	}
	for u.wave.Clicked(gtx) {
		u.chooseWave()
	}

	for i := range u.rows {
		if u.disc.menu != "" {
			break
		}
		if u.rows[i].Clicked(gtx) {
			if u.page == "discovery" {
				u.openExploreItem(i)
				break
			}
			if u.page == "playlists" {
				u.loadPlaylist(u.playlists[i])
				break
			}
			u.feedback("skip")
			u.cancelWaveContinuation()
			u.queue = append([]music.Track(nil), u.tracks...)
			u.queueOffline = u.page == "offline-tracks"
			if u.queueOffline {
				u.offlineQueuePlaylist = u.off.view
			} else {
				u.offlineQueuePlaylist = ""
			}
			u.resetOrder(i)
			u.isWave = u.viewWave && u.session != ""
			if u.viewWave {
				u.activateWaveView()
			}
			u.start(i)
		}
	}
	for u.pause.Clicked(gtx) {
		u.mediaCommand("toggle")
	}
	for u.next.Clicked(gtx) {
		u.advance(false)
	}
	for u.previous.Clicked(gtx) {
		if u.player.Snapshot().Position > 3 {
			u.player.Seek(0)
		} else {
			u.start(u.previousIndex())
		}
	}
	for u.rewind.Clicked(gtx) {
		u.player.Seek(max(0, u.player.Snapshot().Position-15))
	}
	for u.forward.Clicked(gtx) {
		u.player.Seek(min(float64(u.current.DurationMS)/1000-1, u.player.Snapshot().Position+15))
	}
	for u.minus.Clicked(gtx) {
		u.changeVolume(-.1)
	}
	for u.plus.Clicked(gtx) {
		u.changeVolume(.1)
	}
	for u.like.Clicked(gtx) {
		u.likeTrack(u.current)
	}
	u.checkAdvance()
}
func (u *UI) checkAdvance() {
	u.advanceIfEnded(u.player.Snapshot())
}
func (u *UI) advanceIfEnded(s audio.Snapshot) {
	if s.ID == 0 || s.ID != u.playID || u.advancePending || u.stopped || u.pauseRequested || s.Paused {
		return
	}
	if s.Error != "" {
		u.advancePending = true
		if s.DeviceError {
			u.status = s.Error
			u.stopped = true
			u.serviceActive = false
			return
		}
		u.failedTracks++
		// Never refill an exhausted Wave because of failed files, or spin forever
		// through a disconnected service. Manual playback starts a fresh attempt.
		if next := u.nextIndex(); next >= 0 && u.failedTracks < 5 {
			u.startAt(next, 0, false, true)
			u.status = "Недоступный трек пропущен: " + s.Error
		} else {
			u.status = "Воспроизведение остановлено: " + s.Error
			u.stopped = true
			u.serviceActive = false
		}
		return
	}
	if s.Playing && s.Position >= u.requestedOffset+5 {
		u.failedTracks = 0
	}
	if s.Ended && s.ID == u.playID && !u.advancePending {
		u.advancePending = true
		u.advance(true)
	}
}
func (u *UI) backgroundTick() {
	u.recordPlayedHistory()
	u.pollOffline()
	u.checkAdvance()
	u.finishPlaybackLoad()
	u.savePlayback(false)
	u.maybePrefetchWave()
	if runtime.GOOS != "android" {
		return
	}
	s := u.player.Snapshot()
	active := u.serviceActive && u.current.ID != "" && s.Error == ""
	if err := mobile.Publish(mobile.Playback{TrackID: string(u.current.ID), Liked: u.trackLiked(u.current), FeedbackEnabled: u.api != nil, Title: u.current.Title, Artist: u.current.Artist(), Duration: u.current.DurationMS, Position: int64(s.Position * 1000), Playing: s.Playing, Loading: s.Loading && !u.pauseRequested, Active: active}); err != nil {
		u.status = "Не удалось обновить управление Android"
	}
}
func (u *UI) mediaCommand(command string) {
	if u.notificationFeedback(command) {
		return
	}
	s := u.player.Snapshot()
	switch command {
	case "toggle":
		if u.stopped || s.ID == 0 || s.Ended || s.Error != "" {
			u.resumePlayback()
		} else {
			u.pauseRequested = !u.pauseRequested
			if u.pauseRequested {
				u.player.Pause()
			} else {
				u.player.Resume()
			}
		}
	case "pause":
		if u.current.ID != "" && !u.stopped {
			u.pauseRequested = true
			u.player.Pause()
		}
	case "play", "resume":
		u.resumePlayback()
	case "volume-up":
		u.changeVolume(.1)
	case "volume-down":
		u.changeVolume(-.1)
	case "mute":
		u.muteVolume()
	case "next":
		u.advance(false)
	case "previous":
		if s.Position > 3 {
			u.player.Seek(0)
		} else {
			u.start(u.previousIndex())
		}
	case "stop":
		u.cancelWaveContinuation()
		u.checkpointSuspended = false
		u.stopped = true
		u.pauseRequested = true
		u.player.Stop()
		u.serviceActive = false
		u.advancePending = true
	case "task-removed":
		u.suspendPlayback()
		return
	case "service-error":
		u.status = "Android не разрешил фоновое воспроизведение"
	default:
		if strings.HasPrefix(command, "seek:") {
			if ms, e := strconv.ParseInt(strings.TrimPrefix(command, "seek:"), 10, 64); e == nil {
				u.player.Seek(max(0, min(float64(u.current.DurationMS)/1000-.1, float64(ms)/1000)))
			}
		}
	}
	if command == "pause" || command == "toggle" || command == "play" || command == "resume" || command == "stop" {
		u.savePlayback(true)
	}
}
func openBrowser(raw string) {
	if runtime.GOOS == "android" {
		if err := mobile.OpenURL(raw); err != nil {
			log.Print("could not open login page")
		}
		return
	}
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	case "darwin":
		c = exec.Command("open", raw)
	default:
		c = exec.Command("xdg-open", raw)
	}
	if err := c.Start(); err == nil {
		go c.Wait()
	}
}
func (u *UI) label(gtx layout.Context, s string, size unit.Sp, c color.NRGBA) layout.Dimensions {
	l := material.Label(u.textTheme(s), size, s)
	l.Color = c
	return l.Layout(gtx)
}
func (u *UI) button(gtx layout.Context, b *widget.Clickable, s string, primary bool) layout.Dimensions {
	v := material.Button(u.textTheme(s), b, s)
	v.TextSize = unit.Sp(13)
	v.CornerRadius = unit.Dp(10)
	v.Inset = layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(14), Right: unit.Dp(14)}
	if b == &u.wave && primary {
		v.Background = u.waveSourceColor()
	}
	if !primary {
		v.Background = panel
		v.Color = white
	}
	return v.Layout(gtx)
}
func inset(n unit.Dp, w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return layout.UniformInset(n).Layout(gtx, w) }
}
func gap(n unit.Dp) layout.FlexChild { return layout.Rigid(layout.Spacer{Width: n, Height: n}.Layout) }
func fixedHeight(h unit.Dp, w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(h)
		gtx.Constraints.Max.Y = gtx.Dp(h)
		return w(gtx)
	}
}
func (u *UI) layout(gtx layout.Context) layout.Dimensions {
	if u.musifyLists.open {
		return u.musifyListsLayout(gtx)
	}
	if u.musifyLogin.open {
		return u.musifyLoginLayout(gtx)
	}
	u.fit.viewport = gtx.Constraints.Max
	compact := gtx.Constraints.Max.Y < gtx.Dp(680)
	changed := compact != u.fit.compact
	u.fit.compact = compact
	if changed && u.disc.menu == "actions" {
		u.actionsMenu()
	}
	if u.disc.menu != "" && u.disc.menu != "actions" && u.disc.menu != "waves" && u.disc.menu != "services" && u.disc.menu != "accounts" {
		return u.menuLayout(gtx)
	}
	if u.code != "" {
		return u.panelLayout(gtx, u.content)
	}
	if gtx.Constraints.Max.X < gtx.Dp(900) {
		return u.phoneLayout(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(inset(8, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.brand(gtx)
			}), gap(12), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, "МУЗЫКА · ЛЕГЧЕ", 11, muted)
			}), layout.Rigid(u.serviceButton), gap(8), layout.Rigid(u.updateButton), layout.Rigid(u.profileControls))
		})),
		layout.Rigid(inset(8, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return widget.Border{Color: color.NRGBA{R: 55, G: 60, B: 72, A: 255}, CornerRadius: unit.Dp(12), Width: unit.Dp(1)}.Layout(gtx, inset(12, func(gtx layout.Context) layout.Dimensions {
					e := material.Editor(u.textTheme(u.editor.Text()), &u.editor, "Трек или исполнитель")
					e.TextSize = unit.Sp(15)
					return e.Layout(gtx)
				}))
			}), gap(10), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.search, "Найти", true) }))
		})),
		layout.Rigid(inset(8, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !u.hasWaveService() {
					return layout.Dimensions{}
				}
				d := u.button(gtx, &u.wave, u.waveLabel(), true)
				if u.disc.menu == "waves" {
					return u.menuDropdown(gtx, d, true)
				}
				return d
			}), gap(10), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.likes, "Мне нравится", false)
			}), gap(10), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.lists, "Плейлисты", false)
			}), gap(10), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.button(gtx, &u.off.open, "Оффлайн", u.isOfflineView())
			}))
		})),
		layout.Rigid(u.offlineToolbar),
		layout.Rigid(u.discoveryToolbar),
		layout.Rigid(inset(8, func(gtx layout.Context) layout.Dimensions { return u.viewHeading(gtx, 23) })),
		layout.Flexed(1, inset(8, func(gtx layout.Context) layout.Dimensions {
			return u.swipeContent(gtx, u.pagedContent)
		})),
		layout.Rigid(inset(8, func(gtx layout.Context) layout.Dimensions {
			s := u.status
			if p := u.player.Snapshot(); p.Error != "" {
				s = p.Error
			}
			return u.label(gtx, s, 12, muted)
		})),
		layout.Rigid(u.playerBar))
}
func (u *UI) content(gtx layout.Context) layout.Dimensions {
	if u.disc.menu != "" && u.disc.menu != "actions" && u.disc.menu != "waves" && u.disc.menu != "services" && u.disc.menu != "accounts" {
		return u.discoveryMenu(gtx)
	}
	if u.off.picking || u.off.removing != "" || u.page == "offline" {
		return u.offlineContent(gtx)
	}
	if u.up.show {
		return u.updatesContent(gtx)
	}
	if u.code != "" && u.page != "offline-tracks" {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Вход через Яндекс", 20, white)
		}), gap(16), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.label(gtx, u.code, 38, accent) }), gap(16), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Введите этот код на ya.ru/device. Пароль приложению не нужен.", 14, muted)
		}), gap(20), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.button(gtx, &u.open, "Открыть страницу входа", true)
		}))
	}
	if u.busy {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Загружаем музыку…", 18, muted)
		})
	}
	if u.page == "discovery" {
		return u.fitRows(gtx, "discovery:"+u.title, u.exploreCount(), 64, u.exploreRow)
	}
	if u.page == "home" {

		if u.api != nil {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, gap(20), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.label(gtx, "Что включим?", 32, white) }), gap(18), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				text := "Откройте любимые треки, плейлист\nили найдите исполнителя через поиск."
				if u.hasWaveService() {
					text = "Запустите «Мою волну», откройте любимые треки\nили найдите исполнителя через поиск."
				}
				return u.label(gtx, text, 16, muted)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, gap(20), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, "Только вы и музыка.", 32, white)
		}), gap(18), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, u.homeDescription(), 16, muted)
		}), gap(28), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, u.subscriptionDescription(), 13, muted)
		}))
	}
	count := len(u.tracks)
	if u.page == "offline-tracks" && count == 0 {
		return u.label(gtx, "Плейлист пуст. Выберите музыку и нажмите ↓.\nПо умолчанию сохраняем в «Оффлайн».", 15, muted)
	}
	if u.page == "playlists" {
		count = len(u.playlists)
	}
	return u.fitRows(gtx, u.page+":"+u.title, count, 62, u.trackRow)
}

func duration(s float64) string {
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%d:%02d", int(s)/60, int(s)%60)
}
func (u *UI) playerBar(gtx layout.Context) layout.Dimensions {
	return widget.Border{Color: color.NRGBA{R: 45, G: 49, B: 60, A: 255}, Width: unit.Dp(1)}.Layout(gtx, inset(16, func(gtx layout.Context) layout.Dimensions {
		s := u.player.Snapshot()
		title := u.current.Title
		if title == "" {
			title = "Выберите трек"
		}
		artist := u.current.Artist() + u.sourceSuffix(string(u.current.ID)) + u.streamLabel()
		if s.Loading {
			artist = "Буферизация…"
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Body1(u.textTheme(title), title)
					l.MaxLines = 1
					return l.Layout(gtx)
				}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Caption(u.textTheme(artist), artist)
					l.Color = muted
					l.MaxLines = 1
					return l.Layout(gtx)
				}))
			}), gap(8), layout.Rigid(u.shuffleButton), gap(6), layout.Rigid(u.qualityButton), gap(8), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.previous, "Назад", false) }), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := "Играть"
				if s.Playing {
					label = "Пауза"
				}
				return u.button(gtx, &u.pause, label, true)
			}), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.next, "Далее", false) }), gap(12), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.like, "♥", false) }))
		}), gap(12), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			value := float32(0)
			if u.current.DurationMS > 0 {
				value = float32(s.Position / (float64(u.current.DurationMS) / 1000))
			}
			p := material.ProgressBar(u.theme, min(1, value))
			p.Color = accent
			return p.Layout(gtx)
		}), gap(8), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.playbackClock(gtx, s.Position, float64(u.current.DurationMS)/1000, 12)
			}), gap(12), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.rewind, "−15 с", false) }), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.forward, "+15 с", false) }), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.trackActionsButton(gtx)
			}), layout.Flexed(1, layout.Spacer{}.Layout), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.minus, "−", false) }), gap(8), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.label(gtx, u.volumeLabel(), 12, muted)
			}), gap(8), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.plus, "+", false) }), gap(6), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return u.button(gtx, &u.mute, "Mute", u.muted) }))
		}))
	}))
}
