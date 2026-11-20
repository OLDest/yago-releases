package audio

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

func BenchmarkResampler48000(b *testing.B) {
	data := make([]byte, 48000*4)
	dst := make([]byte, 4096)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := newResampler(bytes.NewReader(data), 48000)
		for {
			_, e := r.Read(dst)
			if e == io.EOF {
				break
			}
			if e != nil {
				b.Fatal(e)
			}
		}
	}
}
func BenchmarkCompletedSpoolSmallReads(b *testing.B) {
	f, e := os.CreateTemp(b.TempDir(), "spool")
	if e != nil {
		b.Fatal(e)
	}
	defer f.Close()
	data := make([]byte, 65536)
	if _, e = f.Write(data); e != nil {
		b.Fatal(e)
	}
	s := &Spool{file: f, size: int64(len(data)), done: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dst := make([]byte, 4)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := s.Reader(ctx)
		for {
			_, e := r.Read(dst)
			if e == io.EOF {
				break
			}
			if e != nil {
				b.Fatal(e)
			}
		}
	}
}
