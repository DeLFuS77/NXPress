//go:build cgo

package nup

import (
	"io"

	"github.com/DataDog/zstd"
)

// nativeZstd tells NXPress is built with the zstd library in C.
const nativeZstd = true

// nativeWhole compresses whole files with the zstd library in C, with a context of its own.
type nativeWhole struct {
	ctx   zstd.Ctx
	level int
}

func newNativeWhole(level int) wholeEncoder {
	return nativeWhole{ctx: zstd.NewCtx(), level: level}
}

func (e nativeWhole) encode(src []byte) []byte {
	out, err := e.ctx.CompressLevel(nil, src, e.level)
	if err != nil {
		return nil
	}
	return out
}

func (e nativeWhole) close() {}

// nativeStream compresses a big file with the zstd library in C on threads, a new stream for
// each file: the Go package frees the memory of C on Close and cannot start another stream on
// it. The library writes the same frame for any number of threads from one on (one makes it
// work in the background, as the others), so it is never set to none.
type nativeStream struct {
	level   int
	threads int
	writer  *zstd.Writer
}

func newNativeStream(level int, threads int) streamEncoder {
	return &nativeStream{level: level, threads: threads}
}

func (e *nativeStream) start(w io.Writer, size int64) error {
	e.close()
	e.writer = zstd.NewWriterLevel(&sizedFrame{w: w, size: uint64(size)}, e.level)
	if err := e.writer.SetNbWorkers(e.threads); err != nil {
		e.close()
		return err
	}
	return nil
}

func (e *nativeStream) Write(p []byte) (int, error) {
	return e.writer.Write(p)
}

func (e *nativeStream) finish() error {
	if e.writer == nil {
		return io.ErrClosedPipe
	}
	err := e.writer.Close()
	e.writer = nil
	return err
}

func (e *nativeStream) close() {
	if e.writer != nil {
		e.writer.Close()
		e.writer = nil
	}
}
