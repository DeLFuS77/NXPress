package optimize

import (
	"bufio"
	"encoding/binary"
	"errors"
	"os"
)

// pfsWriter writes an NSP whose file list is known before its files: the head first as zeros,
// the files in order, and the head again at the end, with their sizes.
type pfsWriter struct {
	f       *os.File
	w       *bufio.Writer
	names   []string
	sizes   []int64
	at      int64 // bytes of files written
	head    int64
	current int
}

func pfsHead(names []string, sizes []int64) []byte {
	strs := []byte{}
	offsets := make([]uint32, len(names))
	for i, n := range names {
		offsets[i] = uint32(len(strs))
		strs = append(strs, n...)
		strs = append(strs, 0)
	}
	// the head a whole number of 0x20, as the NSP of Nintendo's tools
	for (0x10+0x18*len(names)+len(strs))%0x20 != 0 {
		strs = append(strs, 0)
	}
	head := make([]byte, 0x10+0x18*len(names))
	copy(head, "PFS0")
	binary.LittleEndian.PutUint32(head[4:], uint32(len(names)))
	binary.LittleEndian.PutUint32(head[8:], uint32(len(strs)))
	off := int64(0)
	for i := range names {
		e := head[0x10+0x18*i:]
		size := int64(0)
		if sizes != nil {
			size = sizes[i]
		}
		binary.LittleEndian.PutUint64(e, uint64(off))
		binary.LittleEndian.PutUint64(e[8:], uint64(size))
		binary.LittleEndian.PutUint32(e[16:], offsets[i])
		off += size
	}
	return append(head, strs...)
}

func newPfsWriter(f *os.File, names []string) (*pfsWriter, error) {
	p := &pfsWriter{f: f, w: bufio.NewWriterSize(f, 4<<20), names: names, sizes: make([]int64, len(names)), current: -1}
	head := pfsHead(names, nil)
	p.head = int64(len(head))
	_, err := p.w.Write(make([]byte, len(head)))
	return p, err
}

// next starts the next file, named as it is now (the name of a content known only once written
// keeps its length: rename sets it).
func (p *pfsWriter) next() { p.current++ }

func (p *pfsWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.sizes[p.current] += int64(n)
	p.at += int64(n)
	return n, err
}

func (p *pfsWriter) rename(i int, name string) error {
	if len(name) != len(p.names[i]) {
		return errors.New("a name of another length")
	}
	p.names[i] = name
	return nil
}

func (p *pfsWriter) finish() error {
	if err := p.w.Flush(); err != nil {
		return err
	}
	head := pfsHead(p.names, p.sizes)
	if int64(len(head)) != p.head {
		return errors.New("the head of the NSP changed size")
	}
	_, err := p.f.WriteAt(head, 0)
	return err
}
