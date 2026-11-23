package offline

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"yago/internal/music"
)

func namedFixture(t *testing.T, l *Library, id, title, codec string) string {
	t.Helper()
	f, e := os.CreateTemp(filepath.Join(l.dir, "audio"), ".download-*.part")
	if e != nil {
		t.Fatal(e)
	}
	b := []byte("fixture")
	f.Write(b)
	f.Close()
	hash := sha256.Sum256(b)
	s := Saved{Track: music.Track{ID: music.ID(id), Title: title, Artists: []music.Named{{Name: "Артист"}}}, Codec: codec, Size: int64(len(b)), SHA256: hex.EncodeToString(hash[:])}
	if e = l.commit("", s, f.Name()); e != nil {
		t.Fatal(e)
	}
	p, _, e := l.Resolve(s.Track.ID)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestReadableAudioNamesCollisionsAndRestart(t *testing.T) {
	l, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p := namedFixture(t, l, "1", "Название", "mp3")
	p2 := namedFixture(t, l, "zvuk-1", "Название", "mp3")
	p3 := namedFixture(t, l, "3", "Название", "flac")
	if filepath.Base(p) != "Артист - Название.mp3" || filepath.Base(p2) != "Артист - Название (2).mp3" || filepath.Ext(p3) != ".flac" {
		t.Fatal(p, p2, p3)
	}
	l, e = Open(l.dir)
	if e != nil {
		t.Fatal(e)
	}
	if q, _, e := l.Resolve("1"); e != nil || q != p {
		t.Fatal("name lost", e)
	}
	if e = l.Remove("", "1"); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("human file not removed")
	}
	if _, _, e = l.Resolve("zvuk-1"); e != nil {
		t.Fatal("collision file removed", e)
	}
	l, e = Open(l.dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = l.Resolve("zvuk-1"); e != nil {
		t.Fatal("garbage removed unrelated file", e)
	}
}
func TestLegacyHashedFileAndUnsafeFilename(t *testing.T) {
	l, _ := Open(t.TempDir())
	p := namedFixture(t, l, "old", "Old", "mp3")
	s := l.data.Tracks["old"]
	s.Filename = ""
	l.data.Tracks["old"] = s
	legacy := l.path("old")
	if e := os.Rename(p, legacy); e != nil {
		t.Fatal(e)
	}
	if e := l.writeLocked(); e != nil {
		t.Fatal(e)
	}
	l, e := Open(l.dir)
	if e != nil {
		t.Fatal(e)
	}
	if q, _, e := l.Resolve("old"); e != nil || q != legacy {
		t.Fatal("legacy file lost", e)
	}
	for _, v := range []string{"../outside.mp3", `..\outside.flac`, "C:outside.mp3", "a\x00.mp3"} {
		if validAudioName(v) {
			t.Fatal("unsafe name accepted", v)
		}
	}
	p = namedFixture(t, l, "bad", `../Title: A?*"<>|`+strings.Repeat("д", 300), "flac")
	if !validAudioName(filepath.Base(p)) || len(filepath.Base(p)) > 200 || filepath.Dir(p) != filepath.Join(l.dir, "audio") {
		t.Fatal("unsafe generated path", p)
	}
}

func TestSavedAudioNameAndBytesNeverChangeWhenMetadataChanges(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		l, e := Open(t.TempDir())
		if e != nil {
			t.Fatal(e)
		}
		path := namedFixture(t, l, "saved", "Original", "mp3")
		if legacy {
			s := l.data.Tracks["saved"]
			s.Filename = ""
			l.data.Tracks["saved"] = s
			old := l.path("saved")
			if e = os.Rename(path, old); e != nil {
				t.Fatal(e)
			}
			path = old
			if e = l.writeLocked(); e != nil {
				t.Fatal(e)
			}
		}
		before, e := os.Stat(path)
		if e != nil {
			t.Fatal(e)
		}
		l, e = Open(l.dir)
		if e != nil {
			t.Fatal(e)
		}
		// The same ID with another title/codec must reuse the existing file.
		got := namedFixture(t, l, "saved", "Changed title", "flac")
		after, e := os.Stat(path)
		if e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if got != path || string(b) != "fixture" || !before.ModTime().Equal(after.ModTime()) {
			t.Fatal("existing saved audio was modified", legacy, got, path)
		}
	}
}
