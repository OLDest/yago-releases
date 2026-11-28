package audio

import (
	"bytes"
	"encoding/binary"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
	"io"
	"runtime"
	"testing"
)

func testFLAC(t *testing.T, bits uint8) ([]byte, []byte) {
	t.Helper()
	var compressed bytes.Buffer
	expected := make([]byte, 1024*4)
	enc, e := flac.NewEncoder(&compressed, &meta.StreamInfo{SampleRate: 44100, NChannels: 2, BitsPerSample: bits, NSamples: 1024, BlockSizeMin: 1024, BlockSizeMax: 1024})
	if e != nil {
		t.Fatal(e)
	}
	f := &frame.Frame{Header: frame.Header{HasFixedBlockSize: true, BlockSize: 1024, SampleRate: 44100, Channels: frame.ChannelsLR, BitsPerSample: bits}}
	for ch := 0; ch < 2; ch++ {
		samples := make([]int32, 1024)
		for i := range samples {
			v := int16(i*13 - 6000 + ch*100)
			samples[i] = int32(v) << (bits - 16)
			binary.LittleEndian.PutUint16(expected[i*4+ch*2:], uint16(v))
		}
		f.Subframes = append(f.Subframes, &frame.Subframe{SubHeader: frame.SubHeader{Pred: frame.PredVerbatim}, Samples: samples, NSamples: 1024})
	}
	if e = enc.WriteFrame(f); e != nil {
		t.Fatal(e)
	}
	if e = enc.Close(); e != nil {
		t.Fatal(e)
	}
	return compressed.Bytes(), expected
}
func TestFLACStreamingPCMAndSeek(t *testing.T) {
	for _, bits := range []uint8{16, 24} {
		b, want := testFLAC(t, bits)
		for _, seek := range []int64{0, 400} {
			d, e := newPCMDecoder(struct{ io.Reader }{bytes.NewReader(b)})
			if e != nil {
				t.Fatal(e)
			}
			if d.SampleRate() != 44100 || d.Length() != 4096 {
				t.Fatal("format")
			}
			if _, e = d.Seek(seek, io.SeekStart); e != nil {
				t.Fatal(e)
			}
			got, e := io.ReadAll(d)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(got, want[seek:]) {
				t.Fatal("PCM differs", bits, seek)
			}
		}
	}
}

type zeroAudioReader struct{}

func (zeroAudioReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestFLACArtworkIsSkippedWithoutRetention(t *testing.T) {
	raw, want := testFLAC(t, 16)
	prefix := append([]byte(nil), raw[:42]...)
	prefix[4] &= 0x7f
	source := io.MultiReader(bytes.NewReader(prefix), bytes.NewReader([]byte{0x86, 0x80, 0, 0}), io.LimitReader(zeroAudioReader{}, 8<<20), bytes.NewReader(raw[42:]))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	d, e := newPCMDecoder(source)
	if e != nil {
		t.Fatal(e)
	}
	got, e := io.ReadAll(d)
	runtime.ReadMemStats(&after)
	if e != nil || !bytes.Equal(got, want) {
		t.Fatal("artwork damaged PCM", e)
	}
	if after.TotalAlloc-before.TotalAlloc > 2<<20 {
		t.Fatal("artwork allocated into heap", after.TotalAlloc-before.TotalAlloc)
	}
}
