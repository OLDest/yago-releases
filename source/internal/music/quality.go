package music

import (
	"errors"
	"sort"
)

var ErrQualityUnavailable = errors.New("Сервис не предоставил MP3 192 кбит/с для этого трека. Выберите HQ")

type Quality int

const (
	Low      Quality = 128
	Normal   Quality = 192
	High     Quality = 320
	Lossless Quality = 1000
)

func NormalizeQuality(q Quality) Quality {
	switch q {
	case 32, 40, 48, 56, 64, 80, 96, 112, Low, 160, Normal, 224, 256, High, Lossless:
		return q
	}
	return High
}

type StreamInfo struct {
	Codec     string
	URL       string
	Bitrate   int
	Available []Quality
	Fallback  bool
}
type downloadVariant struct {
	Codec   string `json:"codec"`
	Bitrate int    `json:"bitrateInKbps"`
	Preview bool   `json:"preview"`
	URL     string `json:"downloadInfoUrl"`
}

func candidates(v []downloadVariant, q Quality) []downloadVariant {
	out := make([]downloadVariant, 0, len(v))
	for _, x := range v {
		if !x.Preview && x.Codec == "mp3" && x.Bitrate > 0 && x.Bitrate <= int(NormalizeQuality(q)) && Trusted(x.URL) {
			out = append(out, x)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Bitrate > out[j].Bitrate })
	return out
}

func availableQualities(v []downloadVariant) []Quality {
	seen := map[Quality]bool{}
	out := []Quality{}
	for _, x := range candidates(v, High) {
		q := Quality(x.Bitrate)
		if NormalizeQuality(q) == q && !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// High is the automatic best-MP3 preference, not a promise of 320 kbit/s.
func NextAvailable(q Quality, available []Quality) Quality {
	if len(available) < 2 {
		return High
	}
	if q == High {
		return available[0]
	}
	for i, v := range available {
		if v == q && i+1 < len(available) {
			return available[i+1]
		}
	}
	return High
}
func ContainsQuality(available []Quality, q Quality) bool {
	for _, v := range available {
		if v == q {
			return true
		}
	}
	return false
}
