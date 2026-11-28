package offline

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"yago/internal/music"
)

func savedFixture(t *testing.T, l *Library, id music.ID) Saved {
	t.Helper()
	b := []byte("local fixture")
	f, e := os.CreateTemp(filepath.Join(l.dir, "audio"), ".download-*.part")
	if e != nil {
		t.Fatal(e)
	}
	f.Write(b)
	f.Close()
	sum := sha256.Sum256(b)
	s := Saved{Track: music.Track{ID: id, Title: "Song", CoverURI: "https://not-persisted.example/cover"}, Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:]), Bitrate: 192, Quality: music.Normal}
	if e = l.commit("", s, f.Name()); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestRepairExistingMissingFilePersistsNewMetadata(t *testing.T) {
	l, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	saved := savedFixture(t, l, "one")
	os.Remove(l.path("one"))
	b := []byte("repaired local data")
	f, e := os.CreateTemp(filepath.Join(l.dir, "audio"), ".download-*.part")
	if e != nil {
		t.Fatal(e)
	}
	f.Write(b)
	f.Close()
	sum := sha256.Sum256(b)
	saved.Size = int64(len(b))
	saved.SHA256 = hex.EncodeToString(sum[:])
	if e = l.commit(DefaultID, saved, f.Name()); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(l.dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, entry, e := reopened.Resolve("one"); e != nil || entry.Size != int64(len(b)) {
		t.Fatal(entry, e)
	}
	if e = l.encodeLocked(&manifestWriter{w: io.Discard, left: 20}); e == nil {
		t.Fatal("manifest byte limit not enforced")
	}
}
func TestDefaultPlaylistSharedFileAndRestart(t *testing.T) {
	dir := t.TempDir()
	l, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	if p := l.Summaries(); len(p) != 1 || p[0].ID != DefaultID || p[0].Name != "Оффлайн" {
		t.Fatal(p)
	}
	custom, e := l.Create("  Дорога  ")
	if e != nil {
		t.Fatal(e)
	}
	savedFixture(t, l, "../../safe-id")
	path, _, e := l.Resolve("../../safe-id")
	if e != nil {
		t.Fatal(e)
	}
	if filepath.Dir(path) != filepath.Join(dir, "audio") {
		t.Fatal("escaped library", path)
	}
	if e = l.Link(custom, "../../safe-id"); e != nil {
		t.Fatal(e)
	}
	if e = l.Link(custom, "../../safe-id"); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	if len(reopened.Tracks(custom)) != 1 || len(reopened.Tracks("")) != 1 {
		t.Fatal("playlist links lost")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "library.json"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "not-persisted") {
		t.Fatal("unused URL retained")
	}
	if e = reopened.Remove(DefaultID, "../../safe-id"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = reopened.Resolve("../../safe-id"); e != nil {
		t.Fatal("shared file deleted", e)
	}
	if e = reopened.Remove(custom, "../../safe-id"); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("unreferenced file remains", e)
	}
	if _, e = Open(dir); e != nil {
		t.Fatal(e)
	}
}
func TestCorruptManifestDoesNotOverwriteData(t *testing.T) {
	dir := t.TempDir()
	l, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	savedFixture(t, l, "one")
	path, _, _ := l.Resolve("one")
	bad := []byte("{broken")
	os.WriteFile(filepath.Join(dir, "library.json"), bad, 0600)
	if _, e = Open(dir); e == nil {
		t.Fatal("corruption ignored")
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal("audio destroyed", e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "library.json"))
	if string(b) != string(bad) {
		t.Fatal("manifest replaced")
	}
}
func TestFailedManifestCommitRollsBack(t *testing.T) {
	dir := t.TempDir()
	l, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	os.Mkdir(filepath.Join(dir, "library.json"), 0700)
	f, _ := os.CreateTemp(filepath.Join(dir, "audio"), ".download-*.part")
	f.Write([]byte("data"))
	f.Close()
	s := Saved{Track: music.Track{ID: "one"}, Size: 4, SHA256: strings.Repeat("a", 64)}
	if e = l.commit("", s, f.Name()); e == nil {
		t.Fatal("commit should fail")
	}
	if len(l.Tracks("")) != 0 {
		t.Fatal("partial entry published")
	}
	if _, e = os.Stat(l.path("one")); !os.IsNotExist(e) {
		t.Fatal("uncommitted MP3 remains")
	}
}
func TestNamesAndInterruptedParts(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir)
	for _, name := range []string{" ", strings.Repeat("я", 81)} {
		if _, e := l.Create(name); e == nil {
			t.Fatal("invalid name accepted")
		}
	}
	if _, e := l.Create("Дорога"); e != nil {
		t.Fatal(e)
	}
	if _, e := l.Create("дорога"); e == nil {
		t.Fatal("duplicate accepted")
	}
	part := filepath.Join(dir, "audio", ".download-interrupted.part")
	os.WriteFile(part, []byte("partial"), 0600)
	savedFixture(t, l, "one")
	if _, e := Open(dir); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(part); !os.IsNotExist(e) {
		t.Fatal("partial remains")
	}
	if _, _, e := l.Resolve("one"); e != nil {
		t.Fatal("saved file removed")
	}
}
