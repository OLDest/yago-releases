// Package updates verifies signed YaGo releases before downloading executable code.
package updates

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const Version = "0.11.12"
const Repository = "OLDest/yago-releases"
const maxBinary = 128 << 20

// PublicKey is pinned in the executable. The signing key is never distributed.
const PublicKey = "2wg1uKVpI1ZokABNuJZQ1/vS/aMEi21WgwI6vTP65n8="

type Artifact struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Schema    int                 `json:"schema"`
	Repo      string              `json:"repo"`
	Version   string              `json:"version"`
	Artifacts map[string]Artifact `json:"artifacts"`
	Notes     []string            `json:"notes,omitempty"`
}
type envelope struct {
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}
type Asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}
type Release struct {
	Tag    string  `json:"tag_name"`
	Draft  bool    `json:"draft"`
	Assets []Asset `json:"assets"`
}
type Available struct {
	Manifest Manifest
	Signed   []byte
	Asset    Asset
	Artifact Artifact
}
type Client struct {
	verifyManifest func([]byte) (Manifest, error)
	Token          string
	HTTP           *http.Client
}

var ErrRepository = errors.New("Репозиторий обновлений недоступен. Повторите позже.")

func New(token string) *Client {
	return &Client{Token: token, HTTP: &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		h := req.URL.Hostname()
		if req.URL.Scheme != "https" || !(h == "api.github.com" || h == "release-assets.githubusercontent.com" || h == "objects.githubusercontent.com") {
			return errors.New("untrusted download redirect")
		}
		req.Header.Del("Authorization")
		return nil
	}}}
}
func numbers(v string) ([3]int, error) {
	var n [3]int
	p := strings.Split(v, ".")
	if len(p) != 3 {
		return n, errors.New("invalid version")
	}
	for i, s := range p {
		if s == "" || len(s) > 8 || (len(s) > 1 && s[0] == '0') {
			return n, errors.New("invalid version")
		}
		for _, r := range s {
			if r < '0' || r > '9' {
				return n, errors.New("invalid version")
			}
		}
		x, e := strconv.Atoi(s)
		if e != nil {
			return n, e
		}
		n[i] = x
	}
	return n, nil
}
func Newer(a, b string) bool {
	x, e := numbers(a)
	if e != nil {
		return false
	}
	y, e := numbers(b)
	if e != nil {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}
func Verify(raw []byte) (Manifest, error) {
	k, _ := base64.StdEncoding.DecodeString(PublicKey)
	return verify(raw, ed25519.PublicKey(k))
}
func verify(raw []byte, key ed25519.PublicKey) (Manifest, error) {
	var m Manifest
	var e envelope
	if len(raw) > 32768 || json.Unmarshal(raw, &e) != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, e.Payload, e.Signature) {
		return m, errors.New("Неверная подпись обновления")
	}
	if json.Unmarshal(e.Payload, &m) != nil || m.Schema != 1 || m.Repo != Repository {
		return m, errors.New("Неверный манифест обновления")
	}
	if _, err := numbers(m.Version); err != nil {
		return m, err
	}
	for platform, a := range m.Artifacts {
		expected := map[string]string{"windows_amd64": "YaGo.exe", "android_universal": "YaGo-android.apk"}[platform]
		hash, err := hex.DecodeString(a.SHA256)
		if expected == "" || a.Name != expected || a.Size <= 0 || a.Size > maxBinary || err != nil || len(hash) != 32 {
			return m, errors.New("Неверные параметры файла обновления")
		}
	}
	return m, nil
}
func (c *Client) request(ctx context.Context, raw, accept string) (*http.Response, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "api.github.com" || !strings.HasPrefix(u.Path, "/repos/"+Repository+"/") {
		return nil, errors.New("untrusted update URL")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "YaGo/"+Version)
	req.Header.Set("Cache-Control", "no-cache")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, e := c.HTTP.Do(req)
	if e != nil {
		return nil, errors.New("Не удалось соединиться с GitHub")
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		if resp.StatusCode == 401 || resp.StatusCode == 404 {
			return nil, ErrRepository
		}
		return nil, fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}
	return resp, nil
}
func readLimited(r io.Reader, n int64) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, n+1))
	if e == nil && int64(len(b)) > n {
		e = errors.New("response too large")
	}
	return b, e
}
func (c *Client) asset(ctx context.Context, id int64) (*http.Response, error) {
	if id <= 0 {
		return nil, errors.New("invalid asset ID")
	}
	return c.request(ctx, fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", Repository, id), "application/octet-stream")
}
func (c *Client) Check(ctx context.Context, current, platform string) (*Available, error) {
	resp, e := c.request(ctx, "https://api.github.com/repos/"+Repository+"/releases?per_page=20", "application/vnd.github+json")
	if e != nil {
		return nil, e
	}
	b, e := readLimited(resp.Body, 2<<20)
	resp.Body.Close()
	if e != nil {
		return nil, e
	}
	var releases []Release
	if json.Unmarshal(b, &releases) != nil {
		return nil, errors.New("Неверный ответ GitHub")
	}
	var best *Available
	for _, r := range releases {
		v := strings.TrimPrefix(r.Tag, "v")
		if r.Draft || !Newer(v, current) || (best != nil && !Newer(v, best.Manifest.Version)) {
			continue
		}
		for _, a := range r.Assets {
			if a.Name != "updates.json" {
				continue
			}
			res, err := c.asset(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			raw, err := readLimited(res.Body, 32768)
			res.Body.Close()
			if err != nil {
				return nil, err
			}
			m, err := c.verify(raw)
			if err != nil {
				return nil, err
			}
			if m.Version != v {
				return nil, errors.New("Версия релиза не совпадает с подписью")
			}
			artifact, exists := m.Artifacts[platform]
			if !exists {
				continue
			}
			for _, binary := range r.Assets {
				if binary.Name == artifact.Name && binary.Size == artifact.Size {
					best = &Available{m, raw, binary, artifact}
				}
			}
		}
	}
	return best, nil
}
func VerifyFile(path string, a Artifact) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() != a.Size {
		return errors.New("Неверный размер обновления")
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, a.Size+1))
	if e != nil {
		return e
	}
	if n != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return errors.New("Контрольная сумма обновления не совпала")
	}
	return nil
}
func (c *Client) Download(ctx context.Context, a *Available, dir string, progress func(int64, int64)) (string, error) {
	// Revalidate signed input at the download boundary.
	m, e := c.verify(a.Signed)
	if e != nil {
		return "", e
	}
	valid := false
	for _, v := range m.Artifacts {
		if v == a.Artifact {
			valid = true
		}
	}
	if !valid {
		return "", errors.New("unsigned artifact")
	}
	dir = filepath.Join(dir, m.Version)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	dest := filepath.Join(dir, a.Artifact.Name)
	if VerifyFile(dest, a.Artifact) == nil {
		if e = os.WriteFile(filepath.Join(dir, "updates.json"), a.Signed, 0600); e != nil {
			return "", e
		}
		return dest, nil
	}
	resp, e := c.asset(ctx, a.Asset.ID)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	f, e := os.CreateTemp(dir, ".download-")
	if e != nil {
		return "", e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 64<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > a.Artifact.Size {
				return "", errors.New("Слишком большой файл обновления")
			}
			if _, e = io.MultiWriter(f, h).Write(buf[:n]); e != nil {
				return "", e
			}
			if progress != nil {
				progress(total, a.Artifact.Size)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.New("Загрузка прервана. Повторите попытку")
		}
	}
	if total != a.Artifact.Size || hex.EncodeToString(h.Sum(nil)) != a.Artifact.SHA256 {
		return "", errors.New("Контрольная сумма обновления не совпала")
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	if e = os.Rename(tmp, dest); e != nil {
		return "", e
	}
	if e = os.WriteFile(filepath.Join(dir, "updates.json"), a.Signed, 0600); e != nil {
		return "", e
	}
	return dest, nil
}

func (c *Client) verify(raw []byte) (Manifest, error) {
	if c.verifyManifest != nil {
		return c.verifyManifest(raw)
	}
	return Verify(raw)
}
