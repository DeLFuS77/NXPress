package nup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// A NUP may be kept split in parts for a FAT32 card, as an NSP is: a folder named like the file,
// with the archive bit, holding the parts 00, 01... in their order. It reads as the NUP whole.

// readerAtCloser is a file open to read anywhere in it.
type readerAtCloser interface {
	io.ReaderAt
	io.Closer
}

// parts is a file split in parts, all of them open, read as one: by several readers at once, as
// a file is.
type parts struct {
	files []*os.File
	// where each part starts in the whole file
	starts []int64
	size   int64
}

func (p *parts) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	read := 0
	for len(b) > 0 {
		if off >= p.size {
			return read, io.EOF
		}
		i := len(p.starts) - 1
		for p.starts[i] > off {
			i--
		}
		end := p.size
		if i+1 < len(p.starts) {
			end = p.starts[i+1]
		}
		n := int(min(int64(len(b)), end-off))
		got, err := p.files[i].ReadAt(b[:n], off-p.starts[i])
		read += got
		off += int64(got)
		b = b[got:]
		if got < n {
			if err == nil || err == io.EOF {
				// a part shorter than it was when it was opened
				err = io.ErrUnexpectedEOF
			}
			return read, err
		}
	}
	return read, nil
}

func (p *parts) Close() error {
	var first error
	for _, file := range p.files {
		if err := file.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// openParts opens the parts 00, 01... of the folder of a file split in parts, up to the first
// one missing.
func openParts(folder string) (*parts, error) {
	p := &parts{}
	for n := 0; ; n++ {
		file, err := os.Open(filepath.Join(folder, fmt.Sprintf("%02d", n)))
		if errors.Is(err, os.ErrNotExist) && n > 0 {
			break
		}
		if err != nil {
			p.Close()
			return nil, err
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			p.Close()
			return nil, err
		}
		if !info.Mode().IsRegular() {
			file.Close()
			p.Close()
			return nil, fmt.Errorf("part %02d is not a file", n)
		}
		p.files = append(p.files, file)
		p.starts = append(p.starts, p.size)
		p.size += info.Size()
	}
	return p, nil
}

// openWhole opens a NUP to read it, or the parts of one split in parts given its folder, and
// tells its size.
func openWhole(path string) (readerAtCloser, int64, error) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		p, err := openParts(path)
		if err != nil {
			return nil, 0, err
		}
		return p, p.size, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, 0, err
	}
	return file, info.Size(), nil
}
