package nup

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"github.com/klauspost/compress/zstd"
)

// The files of a NUP are compressed with the zstd library in C when NXPress is built with it
// (zstdpack_native.go): at the same level it makes them smaller than the encoder in Go, and it
// keeps every core busy on one big file. Without it the encoder in Go does the work. Both write
// standard zstd frames that tell the size of their data, as the format asks (SPEC 4.1), and
// both write the same bytes whatever the number of threads.

// wholeEncoder compresses whole files, the ones a worker reads at once; each worker has one.
type wholeEncoder interface {
	// encode gives src compressed as one frame, nil when it could not be compressed
	encode(src []byte) []byte
	close()
}

// streamEncoder compresses a big file into the NUP as it is read, as one frame, on threads.
type streamEncoder interface {
	// start begins the frame of a file of size bytes, written to w
	start(w io.Writer, size int64) error
	io.Writer
	// finish ends the frame, also after a failed write
	finish() error
	close()
}

// useNative tells whether the zstd library in C compresses the files when NXPress has it; the
// tests turn it off to check the encoder in Go too.
var useNative = nativeZstd

func newWholeEncoder(level int) wholeEncoder {
	if useNative {
		return newNativeWhole(level)
	}
	encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)), zstd.WithEncoderConcurrency(1))
	return goWhole{encoder}
}

// newStreamEncoder gives the encoder of big files on threads; the frame it writes is the same
// for any number of them.
func newStreamEncoder(level int, threads int) streamEncoder {
	if useNative {
		return newNativeStream(level, max(1, threads))
	}
	encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)), zstd.WithEncoderConcurrency(max(1, threads)))
	return goStream{encoder}
}

type goWhole struct {
	encoder *zstd.Encoder
}

func (e goWhole) encode(src []byte) []byte {
	return e.encoder.EncodeAll(src, make([]byte, 0, len(src)/2))
}

func (e goWhole) close() {
	e.encoder.Close()
}

type goStream struct {
	encoder *zstd.Encoder
}

func (e goStream) start(w io.Writer, size int64) error {
	e.encoder.ResetContentSize(w, size)
	return nil
}

func (e goStream) Write(p []byte) (int, error) {
	return e.encoder.Write(p)
}

func (e goStream) finish() error {
	return e.encoder.Close()
}

func (e goStream) close() {
	e.encoder.Close()
}

// errFrameHeader is a frame of the zstd library in C whose header is not the one sizedFrame
// knows how to complete.
var errFrameHeader = errors.New("zstd wrote a frame header NXPress does not expect")

// sizedFrame passes on a zstd frame made without the size of its data, with the size added to
// its header: the Go package of the zstd library in C does not take the size of a stream. The
// header of such a frame is its magic, its descriptor and its window (6 bytes); the one passed
// on tells the size after them, which zstd readers check at the end of the frame.
type sizedFrame struct {
	w    io.Writer
	size uint64
	head []byte
	done bool
}

func (f *sizedFrame) Write(p []byte) (int, error) {
	n := len(p)
	if !f.done {
		take := min(6-len(f.head), len(p))
		f.head = append(f.head, p[:take]...)
		p = p[take:]
		if len(f.head) < 6 {
			return n, nil
		}
		header, err := withSize(f.head, f.size)
		if err != nil {
			return 0, err
		}
		if _, err := f.w.Write(header); err != nil {
			return 0, err
		}
		f.done = true
	}
	if len(p) > 0 {
		if _, err := f.w.Write(p); err != nil {
			return 0, err
		}
	}
	return n, nil
}

// withSize gives the 6 bytes of header of a zstd frame that does not tell its size, telling it:
// in 4 bytes, or 8 for a file of 4 GiB or more.
func withSize(head []byte, size uint64) ([]byte, error) {
	descriptor := head[4]
	// no size, not a single segment, no dictionary, the reserved and unused bits clear; only
	// the checksum may be on
	if binary.LittleEndian.Uint32(head) != zstdMagic || descriptor&^0x04 != 0 {
		return nil, errFrameHeader
	}
	header := make([]byte, 0, 14)
	header = append(header, head[:4]...)
	if size < 1<<32 {
		header = append(header, descriptor|2<<6, head[5])
		return binary.LittleEndian.AppendUint32(header, uint32(size)), nil
	}
	header = append(header, descriptor|3<<6, head[5])
	return binary.LittleEndian.AppendUint64(header, size), nil
}

const zstdMagic = 0xFD2FB528

// compressedStarts are the starts of data that is compressed already, which zstd does not make
// smaller: zstd frames (the .zs files of Nintendo), LZ4 frames, WebM and Matroska, MP4, Ogg,
// PNG, JPEG, gzip and xz.
var compressedStarts = [][]byte{
	{0x28, 0xB5, 0x2F, 0xFD},
	{0x04, 0x22, 0x4D, 0x18},
	{0x1A, 0x45, 0xDF, 0xA3},
	[]byte("OggS"),
	{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A},
	{0xFF, 0xD8, 0xFF},
	{0x1F, 0x8B, 0x08},
	{0xFD, '7', 'z', 'X', 'Z', 0x00},
}

// compressedAlready tells data that is compressed already by its start, so it is stored as it
// is without trying zstd on it.
func compressedAlready(data []byte) bool {
	for _, start := range compressedStarts {
		if bytes.HasPrefix(data, start) {
			return true
		}
	}
	// MP4 and the like: a box "ftyp" first
	return len(data) >= 12 && string(data[4:8]) == "ftyp"
}
