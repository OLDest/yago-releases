package music

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type qualityTransport func(*http.Request) (*http.Response, error)

func (f qualityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func qualityResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func TestQualityVariants(t *testing.T) {
	for _, q := range []Quality{Low, Normal, High} {
		t.Run(strconv.Itoa(int(q)), func(t *testing.T) {
			a := New("secret")
			chosen := ""
			a.HTTP.Transport = qualityTransport(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "download-info") {
					v := []downloadVariant{{"mp3", 320, false, "https://storage.yandex.net/320"}, {"mp3", 128, false, "https://storage.yandex.net/128"}, {"mp3", 192, false, "https://storage.yandex.net/192"}, {"mp3", 128, true, "https://storage.yandex.net/preview"}, {"aac", 128, false, "https://storage.yandex.net/aac"}}
					b, _ := json.Marshal(map[string]any{"result": v})
					return qualityResponse(200, string(b)), nil
				}
				chosen = r.URL.Path
				return qualityResponse(200, "<download-info><host>storage.yandex.net</host><path>/track</path><ts>1</ts><s>s</s></download-info>"), nil
			})
			info, e := a.Stream(context.Background(), Track{ID: "1"}, q)
			if e != nil {
				t.Fatal(e)
			}
			if info.Bitrate != int(q) || chosen != "/"+strconv.Itoa(int(q)) {
				t.Fatalf("bitrate %d path %s", info.Bitrate, chosen)
			}
		})
	}
}
func TestNoPreviewOrAboveLimit(t *testing.T) {
	v := []downloadVariant{{"mp3", 320, false, "https://storage.yandex.net/a"}, {"mp3", 128, true, "https://storage.yandex.net/b"}, {"mp3", 0, false, "https://storage.yandex.net/c"}, {"mp3", 128, false, "https://evil.example/a"}}
	if len(candidates(v, Low)) != 0 {
		t.Fatal("preview, unknown or above-limit variant selected")
	}
	v = append(v, downloadVariant{"mp3", 128, false, "https://storage.yandex.net/d"})
	if got := candidates(v, Normal); len(got) != 1 || got[0].Bitrate != 128 {
		t.Fatal(got)
	}
}
func TestQualityFallbackSignatureAndCap(t *testing.T) {
	for _, tt := range []struct {
		q       Quality
		bitrate int
		mode    string
		wantErr bool
	}{{Low, 128000, "lq", false}, {Normal, 192000, "nq", false}, {Low, 320000, "lq", true}, {Normal, 0, "nq", true}, {High, 0, "nq", false}} {
		a := New("secret")
		a.HTTP.Transport = qualityTransport(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "download-info") {
				return qualityResponse(500, "{}"), nil
			}
			query := r.URL.Query()
			if query.Get("quality") != tt.mode {
				t.Error(query)
			}
			mac := hmac.New(sha256.New, []byte("p93jhgh689SBReK6ghtw62"))
			mac.Write([]byte(query.Get("ts") + "1" + tt.mode + "mp3raw"))
			if query.Get("sign") != base64.RawStdEncoding.EncodeToString(mac.Sum(nil)) {
				t.Error("wrong signature")
			}
			b, _ := json.Marshal(map[string]any{"result": map[string]any{"downloadInfo": map[string]any{"codec": "mp3", "bitrate": tt.bitrate, "url": "https://storage.yandex.net/audio"}}})
			return qualityResponse(200, string(b)), nil
		})
		info, e := a.Stream(context.Background(), Track{ID: "1"}, tt.q)
		if (e != nil) != tt.wantErr {
			t.Fatalf("%+v: %+v %v", tt, info, e)
		}
		if e == nil && info.Bitrate > int(tt.q) {
			t.Fatal("cap exceeded")
		}
	}
}

func TestReportedQualitiesOnly(t *testing.T) {
	v := []downloadVariant{{"mp3", 320, false, "https://storage.yandex.net/full"}, {"mp3", 128, true, "https://storage.yandex.net/preview"}, {"aac", 128, false, "https://storage.yandex.net/aac"}}
	got := availableQualities(v)
	if len(got) != 1 || got[0] != High {
		t.Fatal(got)
	}
	if NextAvailable(High, got) != High {
		t.Fatal("unavailable 128 offered")
	}
	v = append(v, downloadVariant{"mp3", 192, false, "https://storage.yandex.net/192"})
	got = availableQualities(v)
	if len(got) != 2 || NextAvailable(High, got) != Normal || NextAvailable(Normal, got) != High {
		t.Fatal(got)
	}
}
func TestUnavailableSavedQualityFallsBack(t *testing.T) {
	a := New("secret")
	a.HTTP.Transport = qualityTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "download-info") {
			v := []downloadVariant{{"mp3", 320, false, "https://storage.yandex.net/320"}}
			b, _ := json.Marshal(map[string]any{"result": v})
			return qualityResponse(200, string(b)), nil
		}
		return qualityResponse(200, "<download-info><host>storage.yandex.net</host><path>/track</path><ts>1</ts><s>s</s></download-info>"), nil
	})
	info, e := a.Stream(context.Background(), Track{ID: "1"}, Low)
	if e != nil || info.Bitrate != 320 || !info.Fallback || len(info.Available) != 1 || info.Available[0] != High {
		t.Fatalf("%+v %v", info, e)
	}
}

func TestBasicRejectsHigherBitrate(t *testing.T) {
	a := New("secret")
	requests := 0
	a.HTTP.Transport = qualityTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		b, _ := json.Marshal(map[string]any{"result": []downloadVariant{{"mp3", 320, false, "https://storage.yandex.net/320"}}})
		return qualityResponse(200, string(b)), nil
	})
	info, e := a.Stream(context.Background(), Track{ID: "1"}, Normal)
	if !errors.Is(e, ErrQualityUnavailable) || info.URL != "" || requests != 1 {
		t.Fatalf("Basic silently used HQ: %+v %v, %d requests", info, e, requests)
	}
}
