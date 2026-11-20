package audio

import (
	"bytes"
	"github.com/hajimehoshi/go-mp3"
	"io"
	"testing"
)

func taggedData(size int, payload []byte) []byte {
	b := make([]byte, 10+size)
	copy(b, []byte{'I', 'D', '3', 4, 0, 0})
	for i := 9; i >= 6; i-- {
		b[i] = byte(size & 127)
		size >>= 7
	}
	return append(b, payload...)
}
func TestMP3SourceDropsTagsAndSeeksRelativeToAudio(t *testing.T) {
	input := taggedData(100, []byte("audio0123456789"))
	reader, e := newMP3Source(bytes.NewReader(input))
	if e != nil {
		t.Fatal(e)
	}
	s, ok := reader.(io.ReadSeeker)
	if !ok {
		t.Fatal("seek lost")
	}
	b := make([]byte, 5)
	if _, e = io.ReadFull(s, b); e != nil || string(b) != "audio" {
		t.Fatal(string(b), e)
	}
	if pos, e := s.Seek(0, io.SeekCurrent); e != nil || pos != 5 {
		t.Fatal(pos, e)
	}
	if pos, e := s.Seek(2, io.SeekCurrent); e != nil || pos != 7 {
		t.Fatal(pos, e)
	}
	if _, e = s.Read(b[:1]); e != nil || b[0] != '2' {
		t.Fatal(b, e)
	}
	if _, e = s.Seek(0, io.SeekStart); e != nil {
		t.Fatal(e)
	}
	all, e := io.ReadAll(s)
	if e != nil || string(all) != "audio0123456789" {
		t.Fatal(string(all), e)
	}
	if pos, e := s.Seek(-2, io.SeekEnd); e != nil || pos != 13 {
		t.Fatal(pos, e)
	}
	all, e = io.ReadAll(s)
	if e != nil || string(all) != "89" {
		t.Fatal(string(all), e)
	}
}
func TestMP3SourceProgressiveAndMalformedTags(t *testing.T) {
	data := taggedData(1024, []byte("payload"))
	r, e := newMP3Source(bytes.NewBuffer(data))
	if e != nil {
		t.Fatal(e)
	}
	if _, ok := r.(io.Seeker); ok {
		t.Fatal("progressive source falsely seekable")
	}
	b, e := io.ReadAll(r)
	if e != nil || string(b) != "payload" {
		t.Fatal(string(b), e)
	}
	for _, bad := range [][]byte{[]byte("ID3"), {'I', 'D', '3', 4, 0, 0, 128, 0, 0, 0}, taggedData(1024, nil)[:30]} {
		if _, e := newMP3Source(bytes.NewReader(bad)); e == nil {
			t.Fatal("invalid tag accepted")
		}
	}
}
func BenchmarkSkipLargeID3(b *testing.B) {
	data := taggedData(8<<20, []byte("payload"))
	dst := make([]byte, 7)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, e := newMP3Source(bytes.NewReader(data))
		if e != nil {
			b.Fatal(e)
		}
		if _, e = io.ReadFull(r, dst); e != nil {
			b.Fatal(e)
		}
	}
}
func TestRealMP3TaggedSourceDecodesAndSeeks(t *testing.T) {
	data := taggedData(1<<20, fixture(t))
	source, e := newMP3Source(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	decoder, e := mp3.NewDecoder(source)
	if e != nil {
		t.Fatal(e)
	}
	if decoder.Length() < 4 {
		t.Fatal("index lost")
	}
	target := int64(decoder.SampleRate() * 4)
	if _, e = decoder.Seek(target, io.SeekStart); e != nil {
		t.Fatal(e)
	}
	if _, e = io.ReadFull(decoder, make([]byte, 4096)); e != nil {
		t.Fatal(e)
	}
}
