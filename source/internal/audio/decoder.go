package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/hajimehoshi/go-mp3"
	"github.com/mewkiz/flac"
	"io"
)

type pcmDecoder interface {
	io.Reader
	io.Seeker
	SampleRate() int
	Length() int64
}

func newPCMDecoder(r io.Reader) (pcmDecoder, error) {
	source, e := newMP3Source(r)
	if e != nil {
		return nil, e
	}
	var magic [4]byte
	if _, e = io.ReadFull(source, magic[:]); e != nil {
		return nil, e
	}
	if rs, ok := source.(io.ReadSeeker); ok {
		if _, e = rs.Seek(-4, io.SeekCurrent); e != nil {
			return nil, e
		}
	} else {
		source = io.MultiReader(bytes.NewReader(magic[:]), source)
	}
	if string(magic[:]) != "fLaC" {
		return mp3.NewDecoder(source)
	}
	s, e := flac.New(source)
	if e != nil {
		return nil, e
	}
	if s.Info.NChannels < 1 || s.Info.NChannels > 2 || s.Info.SampleRate == 0 || s.Info.SampleRate > 192000 || s.Info.BitsPerSample < 8 || s.Info.BitsPerSample > 24 {
		return nil, errors.New("Неподдерживаемый формат FLAC")
	}
	return &flacPCM{stream: s}, nil
}

// Keep one FLAC frame and a reusable interleaved buffer. Artwork is skipped by
// flac.New. The existing output is stereo 16-bit / 44.1 kHz, also for 24-bit input.
type flacPCM struct {
	stream   *flac.Stream
	pcm      []byte
	at       int
	position int64
}

func (d *flacPCM) SampleRate() int { return int(d.stream.Info.SampleRate) }
func (d *flacPCM) Length() int64   { return int64(d.stream.Info.NSamples) * 4 }
func (d *flacPCM) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if d.at == len(d.pcm) {
		f, e := d.stream.ParseNext()
		if e != nil {
			return 0, e
		}
		if len(f.Subframes) < 1 || len(f.Subframes) > 2 {
			return 0, errors.New("Неверный FLAC кадр")
		}
		n := len(f.Subframes[0].Samples)
		if n == 0 || n > 65535 {
			return 0, errors.New("Неверный размер FLAC кадра")
		}
		if len(f.Subframes) > 1 && len(f.Subframes[1].Samples) != n {
			return 0, errors.New("Неверные каналы FLAC")
		}
		if cap(d.pcm) < n*4 {
			d.pcm = make([]byte, n*4)
		} else {
			d.pcm = d.pcm[:n*4]
		}
		d.at = 0
		bits := f.BitsPerSample
		if bits == 0 {
			bits = d.stream.Info.BitsPerSample
		}
		if bits < 8 || bits > 24 {
			return 0, errors.New("Неверная разрядность FLAC")
		}
		for i := 0; i < n; i++ {
			for ch := 0; ch < 2; ch++ {
				v := f.Subframes[min(ch, len(f.Subframes)-1)].Samples[i]
				if bits > 16 {
					v >>= bits - 16
				} else {
					v <<= 16 - bits
				}
				binary.LittleEndian.PutUint16(d.pcm[i*4+ch*2:], uint16(int16(v)))
			}
		}
	}
	n := copy(p, d.pcm[d.at:])
	d.at += n
	d.position += int64(n)
	return n, nil
}
func (d *flacPCM) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart || offset < d.position {
		return d.position, errors.New("FLAC: обратная перемотка требует нового декодера")
	}
	_, e := io.CopyN(io.Discard, d, offset-d.position)
	return d.position, e
}
