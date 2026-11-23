package updates

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, b []byte) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(b))}
}
func fixture(t *testing.T) (*Client, *Available, ed25519.PrivateKey, []byte) {
	t.Helper()
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	data := []byte("verified binary fixture")
	h := sha256.Sum256(data)
	a := Artifact{"YaGo.exe", int64(len(data)), hex.EncodeToString(h[:])}
	m := Manifest{1, Repository, "0.2.1", map[string]Artifact{"windows_amd64": a}}
	raw := sign(t, m, key)
	c := New("secret-token")
	c.verifyManifest = func(raw []byte) (Manifest, error) { return verify(raw, pub) }
	c.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Error("missing request boundary")
		}
		return response(200, data), nil
	})
	return c, &Available{m, raw, Asset{2, a.Name, a.Size}, a}, key, data
}
func sign(t *testing.T, m Manifest, key ed25519.PrivateKey) []byte {
	t.Helper()
	p, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(envelope{p, ed25519.Sign(key, p)})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestSignatureAndManifest(t *testing.T) {
	_, a, key, _ := fixture(t)
	pub := key.Public().(ed25519.PublicKey)
	if _, e := verify(a.Signed, pub); e != nil {
		t.Fatal(e)
	}
	bad := append([]byte(nil), a.Signed...)
	bad[len(bad)/2] ^= 1
	if _, e := verify(bad, pub); e == nil {
		t.Fatal("tampering accepted")
	}
	if _, e := Verify(a.Signed); e == nil {
		t.Fatal("test signer accepted by production key")
	}
	for _, kind := range []string{"repo", "path", "size", "hash", "version", "schema"} {
		t.Run(kind, func(t *testing.T) {
			m := a.Manifest
			m.Artifacts = map[string]Artifact{"windows_amd64": a.Artifact}
			switch kind {
			case "repo":
				m.Repo = "evil/repo"
			case "path":
				x := a.Artifact
				x.Name = "../YaGo.exe"
				m.Artifacts["windows_amd64"] = x
			case "size":
				x := a.Artifact
				x.Size = maxBinary + 1
				m.Artifacts["windows_amd64"] = x
			case "hash":
				x := a.Artifact
				x.SHA256 = "ab"
				m.Artifacts["windows_amd64"] = x
			case "version":
				m.Version = "../1"
			case "schema":
				m.Schema = 2
			}
			if _, e := verify(sign(t, m, key), pub); e == nil {
				t.Fatal("invalid signed manifest accepted")
			}
		})
	}
}
func TestVersions(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{{"0.2.0", "0.1.9", true}, {"0.10.0", "0.2.0", true}, {"0.2.0", "0.2.0", false}, {"0.1.9", "0.2.0", false}, {"00.2.1", "0.2.0", false}, {"0.3.0-beta", "0.2.0", false}, {"0.3.0/xx", "0.2.0", false}} {
		if Newer(tt.a, tt.b) != tt.want {
			t.Fatalf("%+v", tt)
		}
	}
}
func TestDownload(t *testing.T) {
	for _, kind := range []string{"valid", "truncated", "oversized", "corrupt", "cancelled", "readerror"} {
		t.Run(kind, func(t *testing.T) {
			c, a, _, data := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
				switch kind {
				case "truncated":
					return response(200, data[:3]), nil
				case "oversized":
					return response(200, append(data, 0)), nil
				case "corrupt":
					return response(200, bytes.Repeat([]byte("x"), len(data))), nil
				case "readerror":
					return nil, errors.New("network failed")
				}
				return response(200, data), nil
			})
			if kind == "cancelled" {
				cancel()
			}
			dir := t.TempDir()
			p, e := c.Download(ctx, a, dir, nil)
			if kind == "valid" {
				if e != nil {
					t.Fatal(e)
				}
				if e = VerifyFile(p, a.Artifact); e != nil {
					t.Fatal(e)
				}
				if _, e = os.Stat(filepath.Join(filepath.Dir(p), "updates.json")); e != nil {
					t.Fatal(e)
				}
			} else {
				if e == nil {
					t.Fatal("invalid download accepted")
				}
				if _, e = os.Stat(filepath.Join(dir, a.Manifest.Version, a.Artifact.Name)); !os.IsNotExist(e) {
					t.Fatal("bad artifact retained")
				}
			}
			matches, _ := filepath.Glob(filepath.Join(dir, "*", ".download-*"))
			if len(matches) != 0 {
				t.Fatal("partial file retained")
			}
		})
	}
}
func TestReleaseSelection(t *testing.T) {
	c, a, key, _ := fixture(t)
	m := a.Manifest
	m.Version = "0.3.0"
	newRaw := sign(t, m, key)
	releases := []Release{{Tag: "v0.2.1", Assets: []Asset{{1, "updates.json", 1000}, a.Asset}}, {Tag: "v0.3.0", Assets: []Asset{{3, "updates.json", 1000}, a.Asset}}, {Tag: "v99.0.0", Draft: true}}
	b, _ := json.Marshal(releases)
	c.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(r.URL.RawQuery, "per_page"):
			return response(200, b), nil
		case strings.HasSuffix(r.URL.Path, "/1"):
			return response(200, a.Signed), nil
		default:
			return response(200, newRaw), nil
		}
	})
	best, e := c.Check(context.Background(), "0.2.0", "windows_amd64")
	if e != nil || best == nil || best.Manifest.Version != "0.3.0" {
		t.Fatalf("%+v %v", best, e)
	}
	best, e = c.Check(context.Background(), "0.3.0", "windows_amd64")
	if e != nil || best != nil {
		t.Fatalf("%+v %v", best, e)
	}
}
func TestCredentialBoundary(t *testing.T) {
	c := New("private")
	var leaked bool
	c.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.github.com" {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://release-assets.githubusercontent.com/asset"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		leaked = r.Header.Get("Authorization") != ""
		return response(200, []byte("ok")), nil
	})
	r, e := c.asset(context.Background(), 1)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if leaked {
		t.Fatal("credential leaked to CDN")
	}
	if _, e = c.request(context.Background(), "https://evil.example/repos/"+Repository+"/releases", "application/json"); e == nil {
		t.Fatal("foreign host allowed")
	}
	req, _ := http.NewRequest("GET", "http://release-assets.githubusercontent.com/a", nil)
	if c.HTTP.CheckRedirect(req, []*http.Request{{}}) == nil {
		t.Fatal("insecure redirect allowed")
	}
	c.HTTP.Transport = transport(func(*http.Request) (*http.Response, error) { return response(404, nil), nil })
	if _, e = c.asset(context.Background(), 1); !errors.Is(e, ErrRepository) {
		t.Fatal(e)
	}
}

func TestAnonymousCheck(t *testing.T) {
	c := New("")
	c.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("public check sent a credential")
		}
		return response(200, []byte("[]")), nil
	})
	a, e := c.Check(context.Background(), "0.2.0", "windows_amd64")
	if e != nil || a != nil {
		t.Fatalf("%v %v", a, e)
	}
}
