package updates

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"yago/internal/mobile"
)

func RunInstaller() bool { return false }
func Install(path, state string) error {
	raw, e := os.ReadFile(filepath.Join(filepath.Dir(path), "updates.json"))
	if e != nil {
		return e
	}
	m, e := Verify(raw)
	if e != nil {
		return e
	}
	a, ok := m.Artifacts["android_universal"]
	if !ok || !Newer(m.Version, Version) {
		return errors.New("Обновление устарело")
	}
	if e = VerifyFile(path, a); e != nil {
		return e
	}
	b, _ := json.Marshal(map[string]string{"path": path, "sha256": a.SHA256})
	_, e = mobile.Call("installUpdate", string(b))
	return e
}
