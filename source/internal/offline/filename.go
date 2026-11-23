package offline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

func readableName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			r = ' '
		}
		if b.Len()+utf8.RuneLen(r) > 160 {
			break
		}
		b.WriteRune(r)
	}
	out := strings.Trim(strings.Join(strings.Fields(b.String()), " "), " .")
	if out == "" {
		out = "Трек"
	}
	stem := strings.ToUpper(strings.Split(out, ".")[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
		out = "_" + out
	}
	return out
}
func validAudioName(s string) bool {
	if len(s) == 0 || len(s) > 200 || !utf8.ValidString(s) || filepath.Base(s) != s || (!strings.HasSuffix(s, ".mp3") && !strings.HasSuffix(s, ".flac")) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return false
		}
	}
	stem := strings.TrimSuffix(s, filepath.Ext(s))
	return stem != "" && strings.Trim(stem, " .") == stem
}
func (l *Library) availableName(saved Saved) (string, error) {
	title := strings.TrimSpace(saved.Track.Title)
	artist := strings.TrimSpace(saved.Track.Artist())
	if artist != "" {
		title = artist + " - " + title
	}
	base := readableName(title)
	ext := ".mp3"
	if saved.Codec == "flac" {
		ext = ".flac"
	}
	reserved := make(map[string]bool)
	for _, s := range l.data.Tracks {
		if s.Filename != "" {
			reserved[strings.ToLower(s.Filename)] = true
		}
	}
	for n := 1; n <= MaxTracks+1; n++ {
		name := base + ext
		if n > 1 {
			name = fmt.Sprintf("%s (%d)%s", base, n, ext)
		}
		if reserved[strings.ToLower(name)] {
			continue
		}
		_, e := os.Lstat(filepath.Join(l.dir, "audio", name))
		if os.IsNotExist(e) {
			return name, nil
		}
		if e != nil {
			return "", e
		}
	}
	return "", errors.New("Не удалось выбрать имя аудиофайла")
}
