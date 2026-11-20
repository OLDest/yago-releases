package music

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"time"
)

var ErrLosslessUnavailable = errors.New("FLAC недоступен для этого трека или подписки. Выберите HQ")

func (a *API) losslessStream(ctx context.Context, t Track) (StreamInfo, error) {
	ts := fmt.Sprint(time.Now().Unix())
	mac := hmac.New(sha256.New, []byte("kzqU4XhfCaY6B6JTHODeq5"))
	mac.Write([]byte(ts + string(t.ID) + "losslessflacraw"))
	q := url.Values{"ts": {ts}, "trackId": {string(t.ID)}, "quality": {"lossless"}, "codecs": {"flac"}, "transports": {"raw"}, "sign": {base64.RawStdEncoding.EncodeToString(mac.Sum(nil))}}
	var data struct {
		DownloadInfo struct {
			URL   string
			URLs  []string
			Codec string
		} `json:"downloadInfo"`
	}
	if e := a.callBounded(ctx, "GET", "/get-file-info?"+q.Encode(), nil, nil, &data, 64<<10); e != nil {
		return StreamInfo{}, e
	}
	if data.DownloadInfo.Codec != "flac" {
		return StreamInfo{}, ErrLosslessUnavailable
	}
	for _, raw := range append([]string{data.DownloadInfo.URL}, data.DownloadInfo.URLs...) {
		if Trusted(raw) {
			return StreamInfo{URL: raw, Codec: "flac", Available: []Quality{High, Lossless}}, nil
		}
	}
	return StreamInfo{}, ErrLosslessUnavailable
}
