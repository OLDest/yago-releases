package audio

import (
	"bufio"
	"errors"
	"io"
)

// The decoder allocates a whole ID3 tag just to skip it. Strip leading tags
// through a small buffer instead; embedded artwork never enters the Go heap.
func newMP3Source(source io.Reader) (io.Reader, error) {
	reader := bufio.NewReaderSize(source, 32<<10)
	var skipped int64
	for {
		prefix, e := reader.Peek(3)
		if e != nil {
			if e == io.EOF {
				break
			}
			return nil, e
		}
		var size int64
		switch string(prefix) {
		case "ID3":
			header, e := reader.Peek(10)
			if e != nil {
				return nil, e
			}
			for _, v := range header[6:10] {
				if v&0x80 != 0 {
					return nil, errors.New("invalid ID3 size")
				}
			}
			size = 10 + (int64(header[6])<<21 | int64(header[7])<<14 | int64(header[8])<<7 | int64(header[9]))
			if header[3] == 4 && header[5]&0x10 != 0 {
				size += 10
			}
		case "TAG":
			size = 128
		default:
			goto tagsDone
		}
		if size > MaxTrackBytes-skipped {
			return nil, errors.New("ID3 tag exceeds track limit")
		}
		if _, e := io.CopyN(io.Discard, reader, size); e != nil {
			return nil, e
		}
		skipped += size
	}
tagsDone:
	if seeker, ok := source.(io.ReadSeeker); ok {
		position, e := seeker.Seek(0, io.SeekCurrent)
		if e != nil {
			return nil, e
		}
		return &bufferedMP3Source{Reader: reader, seeker: seeker, base: position - int64(reader.Buffered())}, nil
	}
	return reader, nil
}

type bufferedMP3Source struct {
	*bufio.Reader
	seeker io.ReadSeeker
	base   int64
}

func (s *bufferedMP3Source) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		if offset < 0 {
			return 0, errors.New("negative seek")
		}
		offset += s.base
	case io.SeekCurrent:
		offset -= int64(s.Buffered())
	case io.SeekEnd:
	default:
		return 0, errors.New("invalid seek origin")
	}
	pos, e := s.seeker.Seek(offset, whence)
	if e != nil {
		return 0, e
	}
	s.Reader.Reset(s.seeker)
	return pos - s.base, nil
}
