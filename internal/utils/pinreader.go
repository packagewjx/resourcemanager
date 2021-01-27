package utils

import (
	"encoding/binary"
	"io"
	"os"
)

func NewPinBinaryReader(file string) (*PinBinaryReader, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	return &PinBinaryReader{f: f}, nil
}

type PinBinaryReader struct {
	f *os.File
}

type PinBinaryRecord struct {
	Tid  int
	List []uint64
}

func (p *PinBinaryReader) AsyncRead() <-chan *PinBinaryRecord {
	ch := make(chan *PinBinaryRecord, 1024)
	go func() {
		buf := make([]byte, 4096)
		var tid uint64
		var list []uint64
		for n, err := p.f.Read(buf); err == nil || (n != 0 && err == io.EOF); n, err = p.f.Read(buf) {
			for i := 0; i < len(buf); i += 8 {
				num := binary.LittleEndian.Uint64(buf[i : i+8])
				if num == 0 {
					ch <- &PinBinaryRecord{
						Tid:  int(tid),
						List: list,
					}
					tid = 0
					list = nil
				} else {
					if tid == 0 {
						tid = num
					} else {
						list = append(list, num)
					}
				}
			}
		}
		_ = p.f.Close()
		close(ch)
	}()

	return ch
}

func (p *PinBinaryReader) ReadAll() map[int][]uint64 {
	ch := p.AsyncRead()
	res := make(map[int][]uint64)
	for record := range ch {
		res[record.Tid] = append(res[record.Tid], record.List...)
	}
	return res
}
