package music

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const Base = "https://api.music.yandex.net"
const clientID = "23cabbbdc6cd418abb4b39c32c41195d"
const clientSecret = "53bc75238f0c4d08a118e51fe9203300"

type ID string

func (id *ID) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*id = ID(s)
		return nil
	}
	if string(b) == "null" {
		*id = ""
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*id = ID(n.String())
	return nil
}

type Named struct {
	ID   ID     `json:"id"`
	Name string `json:"name"`
}
type Album struct {
	ID    ID     `json:"id"`
	Title string `json:"title"`
}
type Track struct {
	ID         ID      `json:"id"`
	Title      string  `json:"title"`
	Artists    []Named `json:"artists"`
	Albums     []Album `json:"albums"`
	DurationMS int64   `json:"durationMs"`

	CoverURI  string `json:"coverUri"`
	Available *bool  `json:"available"`
}

func (t Track) Artist() string {
	a := make([]string, len(t.Artists))
	for i, v := range t.Artists {
		a[i] = v.Name
	}
	return strings.Join(a, ", ")
}
func (t Track) FullID() string {
	if len(t.Albums) > 0 {
		return string(t.ID) + ":" + string(t.Albums[0].ID)
	}
	return string(t.ID)
}

type Playlist struct {
	Kind       ID     `json:"kind"`
	Title      string `json:"title"`
	TrackCount int    `json:"trackCount"`
	Owner      struct {
		UID ID `json:"uid"`
	} `json:"owner"`
}
type Account struct {
	UID         ID     `json:"uid"`
	DisplayName string `json:"displayName"`
	Login       string `json:"login"`
}
type API struct {
	Backend Backend
	Token   string
	HTTP    *http.Client
	Base    string
}

func New(token string) *API {
	return &API{Token: token, Base: Base, HTTP: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("слишком много перенаправлений")
		}
		req.Header.Del("Authorization")
		return nil
	}}}
}
func Trusted(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" {
		return false
	}
	h := u.Hostname()
	return h == "yandex.net" || h == "yandex.ru" || strings.HasSuffix(h, ".yandex.net") || strings.HasSuffix(h, ".yandex.ru")
}
func (a *API) call(ctx context.Context, method, path string, form url.Values, body any, out any) error {
	return a.callBounded(ctx, method, path, form, body, out, 16<<20)
}
func (a *API) callBounded(ctx context.Context, method, path string, form url.Values, body any, out any, limit int64) error {
	raw := a.Base + path
	var r io.Reader
	if form != nil {
		r = strings.NewReader(form.Encode())
	}
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		r = strings.NewReader(string(b))
	}
	req, e := http.NewRequestWithContext(ctx, method, raw, r)
	if e != nil {
		return e
	}
	req.Header.Set("X-Yandex-Music-Client", "YandexMusicAndroid/24023621")
	req.Header.Set("Accept-Language", "ru")
	if strings.HasPrefix(path, "/get-file-info?") && req.URL.Query().Get("quality") == "lossless" {
		req.Header.Set("X-Yandex-Music-Client", "YandexMusicDesktopAppWindows/5.95.0")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Yandex-Music-Client", "YandexMusicDesktopAppWindows/5.126.1")
		req.Header.Set("X-Yandex-Music-Without-Invocation-Info", "1")
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if Trusted(raw) && a.Token != "" {
		req.Header.Set("Authorization", "OAuth "+a.Token)
	}
	resp, e := a.HTTP.Do(req)
	if e != nil {
		return errors.New("нет соединения с Яндексом")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("доступ отклонён (%d): проверьте вход и подписку Плюс", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("Яндекс вернул HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNoContent && out == nil {
		return nil
	}
	return decodeAPIResponse(io.LimitReader(resp.Body, limit), strings.HasPrefix(path, "/rotor/session/"), out)
}
func (a *API) Account(ctx context.Context) (Account, error) {
	if a.Backend != nil {
		return a.Backend.Account(ctx)
	}
	var v struct {
		Account Account `json:"account"`
	}
	e := a.call(ctx, "GET", "/account/status", nil, nil, &v)
	return v.Account, e
}
func (a *API) Search(ctx context.Context, q string) ([]Track, error) {
	if a.Backend != nil {
		return a.Backend.Search(ctx, q)
	}
	var v struct {
		Tracks struct {
			Results []Track `json:"results"`
		} `json:"tracks"`
	}
	e := a.call(ctx, "GET", "/search?type=track&page=0&text="+url.QueryEscape(q), nil, nil, &v)
	return v.Tracks.Results, e
}
func (a *API) Tracks(ctx context.Context, ids []string) ([]Track, error) {
	if a.Backend == nil {
		for _, id := range ids {
			if IsZvuk(ID(id)) {
				return nil, errors.New("Трек принадлежит Звуку")
			}
		}
	}
	if a.Backend != nil {
		return a.Backend.Tracks(ctx, ids)
	}
	var all []Track
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		var chunk []Track
		if e := a.call(ctx, "POST", "/tracks", url.Values{"track-ids": {strings.Join(ids[start:end], ",")}}, nil, &chunk); e != nil {
			return nil, e
		}
		all = append(all, chunk...)
	}
	return all, nil
}
func (a *API) Likes(ctx context.Context, uid ID) ([]Track, error) {
	if a.Backend != nil {
		return a.Backend.Likes(ctx, uid)
	}
	var v struct {
		Library struct {
			Tracks []struct {
				ID      ID `json:"id"`
				AlbumID ID `json:"albumId"`
			} `json:"tracks"`
		} `json:"library"`
	}
	if e := a.call(ctx, "GET", "/users/"+url.PathEscape(string(uid))+"/likes/tracks", nil, nil, &v); e != nil {
		return nil, e
	}
	ids := make([]string, 0, len(v.Library.Tracks))
	for _, t := range v.Library.Tracks {
		s := string(t.ID)
		if t.AlbumID != "" {
			s += ":" + string(t.AlbumID)
		}
		ids = append(ids, s)
	}
	return a.Tracks(ctx, ids)
}
func (a *API) Playlists(ctx context.Context, uid ID) ([]Playlist, error) {
	if a.Backend != nil {
		return a.Backend.Playlists(ctx, uid)
	}
	var v []Playlist
	e := a.call(ctx, "GET", "/users/"+url.PathEscape(string(uid))+"/playlists/list", nil, nil, &v)
	return v, e
}
func (a *API) Playlist(ctx context.Context, uid, kind ID) ([]Track, error) {
	if a.Backend == nil && IsZvuk(kind) {
		return nil, errors.New("Плейлист принадлежит Звуку")
	}
	if a.Backend != nil {
		return a.Backend.Playlist(ctx, uid, kind)
	}
	var v struct {
		Tracks []struct {
			Track   Track `json:"track"`
			ID      ID    `json:"id"`
			AlbumID ID    `json:"albumId"`
		} `json:"tracks"`
	}
	e := a.call(ctx, "GET", "/users/"+url.PathEscape(string(uid))+"/playlists/"+url.PathEscape(string(kind)), nil, nil, &v)
	if e != nil {
		return nil, e
	}
	out := make([]Track, 0, len(v.Tracks))
	for _, t := range v.Tracks {
		if t.Track.ID != "" {
			out = append(out, t.Track)
		}
	}
	return out, nil
}
func (a *API) Like(ctx context.Context, uid ID, t Track) error {
	if a.Backend == nil && IsZvuk(t.ID) {
		return errors.New("Объект принадлежит Звуку. Подключите этот сервис")
	}
	if a.Backend != nil {
		return a.Backend.Like(ctx, uid, t)
	}
	return a.call(ctx, "POST", "/users/"+url.PathEscape(string(uid))+"/likes/tracks/add-multiple", url.Values{"track-ids": {t.FullID()}}, nil, nil)
}

type Wave struct {
	Session  string `json:"radioSessionId"`
	Batch    string `json:"batchId"`
	Sequence []struct {
		Track Track `json:"track"`
	} `json:"sequence"`
}

type WaveContext struct {
	Session string `json:"session"`
	Batch   string `json:"batch"`
}

func (a *API) Wave(ctx context.Context, session string, queue []string) (Wave, error) {
	return a.WaveWithSeeds(ctx, session, queue, nil)
}
func (a *API) WaveWithSeeds(ctx context.Context, session string, queue, seeds []string) (Wave, error) {
	if a.Backend != nil {
		return a.Backend.WaveWithSeeds(ctx, session, queue, seeds)
	}
	if len(seeds) == 0 {
		seeds = []string{"user:onyourwave"}
	}
	path := "/rotor/session/new"
	body := map[string]any{"seeds": seeds, "includeTracksInResponse": true, "includeWaveModel": false, "interactive": true}
	if len(queue) > 0 {
		body["queue"] = queue
	}
	if session != "" {
		path = "/rotor/session/" + url.PathEscape(session) + "/tracks"
		body = map[string]any{"queue": queue}
	}
	var v Wave
	if err := a.call(ctx, "POST", path, nil, body, &v); err != nil {
		return Wave{}, err
	}
	if v.Session == "" {
		v.Session = session
	}
	tracks := v.Sequence[:0]
	for _, item := range v.Sequence {
		if item.Track.ID != "" {
			tracks = append(tracks, item)
		}
	}
	v.Sequence = tracks
	if len(v.Sequence) == 0 {
		return Wave{}, errors.New("волна не вернула треки")
	}
	if v.Session == "" {
		return Wave{}, errors.New("волна не вернула идентификатор сессии")
	}
	return v, nil
}
func (a *API) Feedback(ctx context.Context, session, batch, kind string, t Track, played float64) error {
	if a.Backend != nil {
		return a.Backend.Feedback(ctx, session, batch, kind, t, played)
	}
	event := map[string]any{"type": kind, "timestamp": time.Now().UTC().Format(time.RFC3339Nano)}
	if t.ID != "" {
		event["trackId"] = t.FullID()
		event["totalPlayedSeconds"] = played
	}
	body := map[string]any{"event": event, "from": "desktop_win-home-play"}
	if kind == "radioStarted" {
		event["from"] = "desktop_win-home-play"
	} else {
		body["batchId"] = batch
	}
	return a.call(ctx, "POST", "/rotor/session/"+url.PathEscape(session)+"/feedback/", nil, body, nil)
}
func (a *API) StreamURL(ctx context.Context, t Track) (string, error) {
	info, err := a.Stream(ctx, t, High)
	return info.URL, err
}
func (a *API) Stream(ctx context.Context, t Track, q Quality) (StreamInfo, error) {
	if a.Backend == nil && IsZvuk(t.ID) {
		return StreamInfo{}, errors.New("Трек принадлежит Звуку")
	}
	if a.Backend != nil {
		return a.Backend.Stream(ctx, t, q)
	}
	q = NormalizeQuality(q)
	if q == Lossless {
		return a.losslessStream(ctx, t)
	}
	raw, err := a.legacyStream(ctx, t, q)
	if err == nil {
		return raw, nil
	}
	if errors.Is(err, ErrQualityUnavailable) {
		return raw, err
	}
	if ctx.Err() != nil {
		return StreamInfo{}, ctx.Err()
	}
	quality := "nq"
	if q == Low {
		quality = "lq"
	}
	ts := fmt.Sprint(time.Now().Unix())
	id := string(t.ID)
	mac := hmac.New(sha256.New, []byte("p93jhgh689SBReK6ghtw62"))
	mac.Write([]byte(ts + id + quality + "mp3raw"))
	query := url.Values{"ts": {ts}, "trackId": {id}, "quality": {quality}, "codecs": {"mp3"}, "transports": {"raw"}, "sign": {base64.RawStdEncoding.EncodeToString(mac.Sum(nil))}}
	var result struct {
		DownloadInfo struct {
			URL         string   `json:"url"`
			URLs        []string `json:"urls"`
			Codec       string   `json:"codec"`
			Bitrate     int      `json:"bitrate"`
			BitrateKbps int      `json:"bitrateInKbps"`
		} `json:"downloadInfo"`
	}
	if e := a.call(ctx, "GET", "/get-file-info?"+query.Encode(), nil, nil, &result); e != nil {
		return StreamInfo{}, e
	}
	d := result.DownloadInfo
	if d.Codec != "" && d.Codec != "mp3" {
		return StreamInfo{}, errors.New("сервис не вернул MP3")
	}
	bitrate := d.BitrateKbps
	if bitrate == 0 {
		bitrate = d.Bitrate
		if bitrate > 1000 {
			bitrate /= 1000
		}
	}
	if q == Normal && bitrate != 192 {
		return StreamInfo{}, ErrQualityUnavailable
	}
	if bitrate > int(q) || (bitrate <= 0 && q != High) {
		return StreamInfo{}, errors.New("Сервис не подтвердил выбранный битрейт MP3; выберите более высокое качество")
	}
	urls := append([]string{d.URL}, d.URLs...)
	for _, u := range urls {
		if Trusted(u) {
			var available []Quality
			if bitrate > 0 {
				available = []Quality{Quality(bitrate)}
			}
			return StreamInfo{URL: u, Bitrate: bitrate, Available: available}, nil
		}
	}
	return StreamInfo{}, errors.New("нет безопасного адреса MP3")
}
func (a *API) legacyStream(ctx context.Context, t Track, q Quality) (StreamInfo, error) {
	return a.downloadStream(ctx, t, q)
}

func (a *API) downloadStream(ctx context.Context, t Track, q Quality) (StreamInfo, error) {
	var variants []downloadVariant
	if e := a.call(ctx, "GET", "/tracks/"+url.PathEscape(string(t.ID))+"/download-info", nil, nil, &variants); e != nil {
		return StreamInfo{}, e
	}

	available := availableQualities(variants)
	fallback := q != High && len(available) > 0 && !ContainsQuality(available, q)
	if fallback && q == Normal {
		return StreamInfo{Available: available}, ErrQualityUnavailable
	}
	if fallback {
		q = High
	}
	for _, v := range candidates(variants, q) {
		req, e := http.NewRequestWithContext(ctx, "GET", v.URL, nil)
		if e != nil {
			continue
		}
		if a.Token != "" {
			req.Header.Set("Authorization", "OAuth "+a.Token)
		}
		resp, e := a.HTTP.Do(req)
		if e != nil {
			return StreamInfo{}, errors.New("не удалось получить адрес аудио")
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}
		var x struct {
			Host string `xml:"host"`
			Path string `xml:"path"`
			TS   string `xml:"ts"`
			S    string `xml:"s"`
		}
		e = xml.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&x)
		resp.Body.Close()
		if e != nil || x.Host == "" || !strings.HasPrefix(x.Path, "/") {
			continue
		}
		u := &url.URL{Scheme: "https", Host: x.Host}
		if !Trusted(u.String()) || strings.ContainsAny(x.Host, "/@?#") {
			continue
		}
		sum := md5.Sum([]byte("XGRlBW9FXlekgbPrRHuSiA" + strings.TrimPrefix(x.Path, "/") + x.S))
		return StreamInfo{URL: "https://" + x.Host + "/get-mp3/" + hex.EncodeToString(sum[:]) + "/" + x.TS + x.Path, Bitrate: v.Bitrate, Available: available, Fallback: fallback}, nil
	}
	return StreamInfo{}, errors.New("MP3 недоступен для этого трека")
}

type DeviceCode struct {
	Device   string `json:"device_code"`
	User     string `json:"user_code"`
	Interval int    `json:"interval"`
	Expires  int    `json:"expires_in"`
}
type Token struct {
	Profile *Account `json:"profile,omitempty"`
	Service string   `json:"service,omitempty"`
	Access  string   `json:"access_token"`
	Refresh string   `json:"refresh_token,omitempty"`
	Expires int64    `json:"expires_in,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func oauth(ctx context.Context, path string, form url.Values, out any) error {
	req, e := http.NewRequestWithContext(ctx, "POST", "https://oauth.yandex.ru"+path, strings.NewReader(form.Encode()))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if e != nil {
		return errors.New("нет соединения с сервером входа")
	}
	defer resp.Body.Close()
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(out)
}
func StartLogin(ctx context.Context, device string) (DeviceCode, error) {
	var v DeviceCode
	e := oauth(ctx, "/device/code", url.Values{"client_id": {clientID}, "device_id": {device}, "device_name": {"YaGo Desktop"}}, &v)
	if e == nil && v.Device == "" {
		e = errors.New("Яндекс не выдал код входа")
	}
	return v, e
}
func PollLogin(ctx context.Context, code string) (Token, error) {
	var t Token
	e := oauth(ctx, "/token", url.Values{"grant_type": {"device_code"}, "code": {code}, "client_id": {clientID}, "client_secret": {clientSecret}}, &t)
	return t, e
}
func Refresh(ctx context.Context, refresh string) (Token, error) {
	var t Token
	e := oauth(ctx, "/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}, "client_secret": {clientSecret}}, &t)
	if e == nil && t.Access == "" {
		e = errors.New("нужно войти заново")
	}
	return t, e
}
