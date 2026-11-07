package nup

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/klauspost/compress/zstd"
)

const (
	// the data handed to a worker at once
	chunkSize = 1 << 20
	// the data of small files read at once: the many small files of a game are stored one after
	// the other, and are read in a few big reads instead of one each
	windowSize = 8 << 20
	// the chunks waiting for each worker
	queued = 4
)

// the window of zstd a file may need to be decompressed: level 3, which the packer of the format
// uses, needs 2 MB, the highest level 128 MB. A file asking for more is told as one that cannot
// be decompressed, instead of taking the memory of the computer.
const maxWindow = 128 << 20

// Damaged is a file of a NUP whose data is not what its table of contents says.
type Damaged struct {
	// the package it is in ("game v65536", "update v131072", "DLC 0400...3001 v0"), "" for a
	// file in no folder of any package
	Package string
	// its path inside the package: romfs/Data/stage.bin
	Path   string
	Reason string
	// a code module of an ExeFS whose data is as it was packed, but that its own header tells
	// damaged: the dump the NUP was made of lacks bytes of it (nso.go)
	Code bool
}

// Check is what the verification of a NUP found.
type Check struct {
	// the files checked, and the pieces of data read for them (files with the same data share
	// one, read once)
	Files   int
	Extents int
	// the bytes read from the NUP
	Read    int64
	Damaged []Damaged
	// the code modules of the ExeFS of the packages read by their headers
	Modules int
	// files of the table that no folder of any package holds: checked all the same
	Unplaced int
	// what the metadata of the packages tells otherwise than their files, which the format
	// allows (see checkMeta)
	Notes []string
}

// extentJob is a piece of data of the NUP to read and check, with the files it is the data of.
type extentJob struct {
	offset, stored, size uint64
	codec                byte
	files                []int
	// the files it is written into, when a NUP is unpacked
	outputs []string
	// why the data is damaged, "" when it is whole; and its SHA-256, once read whole
	reason string
	sum    [32]byte
	// the data of a code module, read by its header as it is hashed: whether it starts as one,
	// and what its header tells wrong with it
	code        bool
	module      bool
	codeProblem string
}

// feed is the data of one piece, handed by the reader to a worker chunk after chunk; failed is
// set before chunks is closed when the disk could not read it all.
type feed struct {
	job    *extentJob
	chunks chan []byte
	failed error
}

// Verify reads every piece of data of a NUP once, in the order it is stored, and compares the
// SHA-256 of each file, decompressed when it is stored with zstd, with the one its table of
// contents keeps. Readers go through the file in order, one for a hard disk, which reads fastest
// so, several for an SSD, and workers hash and decompress what they read; nothing is held in
// memory but a few buffers. The header and the table of contents are checked first, by their hash: a NUP whose
// table is damaged fails as a whole, a NUP whose files are damaged gives them in
// Check.Damaged, with the metadata of a package that cannot be what it says (checkMeta). The
// code modules of each ExeFS are read by their headers as they go by (nso.go): the hashes of a
// NUP are of the files as they were packed, and a dump that lost bytes of its code gives a NUP
// whose hashes match but whose game does not start.
// progress is told the bytes read of the ones to read.
func Verify(ctx context.Context, path string, workers int, readers int, progress func(done int64, total int64)) (*Container, Check, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, Check{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, Check{}, err
	}
	container, err := Read(file, info.Size())
	if err != nil {
		return nil, Check{}, err
	}
	check := Check{Files: len(container.Files)}

	// the code modules of the ExeFS of each package
	code := make([]bool, len(container.Files))
	for i := range container.Packages {
		if err := container.Walk(i, func(path string, index int) error {
			code[index] = code[index] || IsCodeModule(path)
			return nil
		}); err != nil {
			return container, check, err
		}
	}

	// the pieces of data to read, each once, in the order they are stored
	type key struct {
		offset, stored, size uint64
		codec                byte
	}
	byKey := map[key]*extentJob{}
	empty := sha256.Sum256(nil)
	reasons := make([]string, len(container.Files))
	for i, f := range container.Files {
		if f.Size == 0 {
			if f.Hash != empty {
				reasons[i] = "its SHA-256 does not match"
			}
			continue
		}
		k := key{f.ExtentOffset, f.ExtentSize, f.Size, f.Codec}
		job, ok := byKey[k]
		if !ok {
			job = &extentJob{offset: f.ExtentOffset, stored: f.ExtentSize, size: f.Size, codec: f.Codec}
			byKey[k] = job
		}
		job.files = append(job.files, i)
		job.code = job.code || code[i]
	}
	jobs := make([]*extentJob, 0, len(byKey))
	total := int64(0)
	for _, job := range byKey {
		jobs = append(jobs, job)
		total += int64(job.stored)
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].offset != jobs[j].offset {
			return jobs[i].offset < jobs[j].offset
		}
		return jobs[i].stored < jobs[j].stored
	})
	check.Extents = len(jobs)
	if progress != nil {
		progress(0, total)
	}

	read, err := readAll(ctx, file, jobs, max(1, workers), max(1, readers), func(done int64) {
		if progress != nil {
			progress(done, total)
		}
	})
	check.Read = read
	if err != nil {
		return container, check, err
	}
	// a module whose data is not what was packed is damaged by its data, not by the dump
	codeReasons := make([]string, len(container.Files))
	for _, job := range jobs {
		for _, index := range job.files {
			switch {
			case job.reason != "":
				reasons[index] = job.reason
			case container.Files[index].Hash != job.sum:
				reasons[index] = "its SHA-256 does not match"
			case code[index] && job.module:
				check.Modules++
				codeReasons[index] = job.codeProblem
			}
		}
	}

	// the damaged files by their path, found in the tree of each package
	placed := make([]bool, len(container.Files))
	for i, pkg := range container.Packages {
		label := Label(pkg)
		err := container.Walk(i, func(path string, index int) error {
			switch {
			case placed[index]:
			case reasons[index] != "":
				check.Damaged = append(check.Damaged, Damaged{Package: label, Path: path, Reason: reasons[index]})
			case codeReasons[index] != "":
				check.Damaged = append(check.Damaged, Damaged{Package: label, Path: path, Reason: codeReasons[index], Code: true})
			}
			placed[index] = true
			return nil
		})
		if err != nil {
			return container, check, err
		}
	}
	for index, reason := range reasons {
		if !placed[index] {
			check.Unplaced++
			if reason != "" {
				check.Damaged = append(check.Damaged, Damaged{Path: container.Files[index].Name, Reason: reason})
			}
		}
	}
	damagedFiles := make([]bool, len(reasons))
	for index, reason := range reasons {
		damagedFiles[index] = reason != ""
	}
	meta, notes := checkMeta(file, container, damagedFiles)
	check.Damaged = append(check.Damaged, meta...)
	check.Notes = notes
	return container, check, nil
}

// readAll reads the pieces of data in their order and hands each to the first worker free; it
// gives the bytes read. Each reader goes through its own stretch of the NUP from start to end:
// one reader for a hard disk, which reads fastest in order, several for an SSD, which reads
// faster with several reads at once. progress is told the bytes read, from one reader at a time.
func readAll(ctx context.Context, file *os.File, jobs []*extentJob, workers int, readers int, progress func(done int64)) (int64, error) {
	workers = min(workers, max(1, len(jobs)))
	readers = max(1, min(readers, workers))
	free := make(chan []byte, workers*(queued+1)+2*readers)
	for range cap(free) {
		free <- make([]byte, chunkSize)
	}
	idle := make(chan int, workers)
	feeds := make([]chan *feed, workers)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wait    sync.WaitGroup
		failure atomic.Value
	)
	fail := func(err error) {
		if failure.CompareAndSwap(nil, err) {
			cancel()
		}
	}
	for w := range workers {
		feeds[w] = make(chan *feed, 1)
		idle <- w
		wait.Add(1)
		go func() {
			defer wait.Done()
			worker := hasher{free: free}
			defer worker.close()
			for f := range feeds[w] {
				if err := worker.check(f); err != nil {
					fail(err)
				}
				idle <- w
			}
		}()
	}

	var (
		done      atomic.Int64
		told      sync.Mutex
		stretches sync.WaitGroup
	)
	tell := func(n int64) {
		value := done.Add(n)
		if told.TryLock() {
			progress(value)
			told.Unlock()
		}
	}
	// the stretches of the readers: the pieces in their order, cut where the bytes are shared
	total := int64(0)
	for _, job := range jobs {
		total += int64(job.stored)
	}
	share, first, sum := total/int64(readers)+1, 0, int64(0)
	for i := range jobs {
		sum += int64(jobs[i].stored)
		if sum >= share || i == len(jobs)-1 {
			stretch := jobs[first : i+1]
			first, sum = i+1, 0
			stretches.Add(1)
			go func() {
				defer stretches.Done()
				if err := readStretch(ctx, file, stretch, free, idle, feeds, tell); err != nil {
					fail(err)
				}
			}()
		}
	}
	stretches.Wait()
	for _, f := range feeds {
		close(f)
	}
	wait.Wait()
	if err, _ := failure.Load().(error); err != nil {
		return done.Load(), err
	}
	return done.Load(), nil
}

// readStretch reads the pieces of one stretch of the NUP in their order, small pieces in a few
// big reads, and hands each to the first worker free. A part of the disk that cannot be read
// makes its piece damaged and the reading goes on; only a cancelled work stops it.
func readStretch(ctx context.Context, file *os.File, jobs []*extentJob, free chan []byte, idle chan int, feeds []chan *feed,
	tell func(int64)) error {
	var window []byte
	windowAt := int64(-1)
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		f := &feed{job: job, chunks: make(chan []byte, queued)}
		var w int
		select {
		case w = <-idle:
		case <-ctx.Done():
			return ctx.Err()
		}
		feeds[w] <- f
		at, end := int64(job.offset), int64(job.offset+job.stored)
		for at < end {
			if err := ctx.Err(); err != nil {
				f.failed = err
				close(f.chunks)
				return err
			}
			buffer := <-free
			n := min(int64(len(buffer)), end-at)
			var err error
			if job.stored < windowSize/2 {
				// small pieces come from the window, read again when the piece goes past it
				if windowAt < 0 || at < windowAt || end > windowAt+int64(len(window)) {
					if window == nil {
						window = make([]byte, windowSize)
					}
					window = window[:cap(window)]
					var got int
					got, err = file.ReadAt(window, at)
					if errors.Is(err, io.EOF) && int64(got) >= end-at {
						err = nil
					}
					window, windowAt = window[:got], at
					if err != nil {
						// a part of the disk that cannot be read: only this piece is read again on
						// its own, so the pieces around it are not told damaged with it
						windowAt = -1
						var got int
						got, err = file.ReadAt(buffer[:n], at)
						if errors.Is(err, io.EOF) && int64(got) == n {
							err = nil
						}
					}
				}
				if err == nil && windowAt >= 0 {
					copy(buffer, window[at-windowAt:at-windowAt+n])
				}
			} else {
				var got int
				got, err = file.ReadAt(buffer[:n], at)
				if errors.Is(err, io.EOF) && int64(got) == n {
					err = nil
				}
			}
			if err != nil {
				free <- buffer
				f.failed = err
				windowAt = -1
				break
			}
			f.chunks <- buffer[:n]
			at += n
			tell(n)
		}
		close(f.chunks)
	}
	return nil
}

// hasher is a worker: it hashes the data of a piece as it comes, decompressed for zstd, and when
// the piece has files to write (unpacking), writes it into each of them.
type hasher struct {
	free    chan []byte
	decoder *zstd.Decoder
	out     []byte
}

func (h *hasher) close() {
	if h.decoder != nil {
		h.decoder.Close()
	}
}

// check hashes the data of a piece, and keeps in the piece its hash or why it is damaged. Its
// chunks are all taken, whatever happens, so the reader never waits for it. It gives an error
// only when the work cannot go on: a file it cannot write.
func (h *hasher) check(f *feed) error {
	job := f.job
	hash := sha256.New()
	source := &chunkReader{chunks: f.chunks, free: h.free}
	defer source.drain()
	var into io.Writer = hash
	if len(job.outputs) > 0 {
		writers := []io.Writer{hash}
		for _, path := range job.outputs {
			out, err := createOutput(path)
			if err != nil {
				return err
			}
			defer out.Close()
			writers = append(writers, out)
		}
		into = &guardedWriter{w: io.MultiWriter(writers...)}
	}
	// a code module is read by its header as it is hashed
	var module *nsoCheck
	if job.code {
		module = newNsoCheck(int64(job.size))
		into = io.MultiWriter(into, module)
	}
	var (
		n   int64
		err error
	)
	if job.codec == CodecZstd {
		if h.decoder == nil {
			if h.decoder, err = zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(maxWindow),
				zstd.WithDecoderMaxMemory(maxWindow)); err != nil {
				return err
			}
			h.out = make([]byte, chunkSize)
		}
		if err = h.decoder.Reset(source); err == nil {
			// one byte more than the file has tells data that goes on past it
			n, err = io.CopyBuffer(into, io.LimitReader(h.decoder, int64(job.size)+1), h.out)
		}
	} else {
		n, err = source.hashAll(into)
	}
	source.drain()
	var written *writeError
	switch {
	case errors.As(err, &written):
		return written.err
	case f.failed != nil:
		job.reason = fmt.Sprintf("its data cannot be read (%v)", f.failed)
	case err != nil:
		job.reason = fmt.Sprintf("its zstd data cannot be decompressed (%v)", err)
	case n > int64(job.size):
		job.reason = fmt.Sprintf("its data decompresses to more than its %d bytes", job.size)
	case n < int64(job.size):
		job.reason = fmt.Sprintf("its data decompresses to %d bytes instead of %d", n, job.size)
	default:
		copy(job.sum[:], hash.Sum(nil))
		if module != nil {
			job.module, job.codeProblem = module.module(), module.result()
		}
	}
	return nil
}

// createOutput makes a new file of an unpacking; the tests make files that fail as a full disk does.
var createOutput = func(path string) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
}

// writeError is a file of an unpacking that could not be written, told apart from damaged data.
type writeError struct {
	err error
}

func (e *writeError) Error() string {
	return e.err.Error()
}

// guardedWriter tells the errors of the files written as writeErrors.
type guardedWriter struct {
	w io.Writer
}

func (g *guardedWriter) Write(p []byte) (int, error) {
	n, err := g.w.Write(p)
	if err != nil {
		return n, &writeError{err}
	}
	return n, nil
}

// chunkReader reads the chunks of a piece as one stream, giving each buffer back once read.
type chunkReader struct {
	chunks <-chan []byte
	free   chan<- []byte
	held   []byte
	rest   []byte
}

func (r *chunkReader) Read(p []byte) (int, error) {
	for len(r.rest) == 0 {
		r.release()
		chunk, ok := <-r.chunks
		if !ok {
			return 0, io.EOF
		}
		r.held, r.rest = chunk, chunk
	}
	n := copy(p, r.rest)
	r.rest = r.rest[n:]
	return n, nil
}

// hashAll writes every chunk into to as it is, without copying it, and gives the bytes.
func (r *chunkReader) hashAll(to io.Writer) (int64, error) {
	n := int64(0)
	for chunk := range r.chunks {
		_, err := to.Write(chunk)
		n += int64(len(chunk))
		r.free <- chunk[:cap(chunk)]
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func (r *chunkReader) release() {
	if r.held != nil {
		r.free <- r.held[:cap(r.held)]
		r.held, r.rest = nil, nil
	}
}

// drain takes the chunks left, when the data stopped being read before its end.
func (r *chunkReader) drain() {
	r.release()
	for chunk := range r.chunks {
		r.free <- chunk[:cap(chunk)]
	}
}

// Label is how a package is told in the paths of its files: "game v65536", "update v131072",
// "DLC 0400245021D2D001 v0".
func Label(pkg Package) string {
	switch pkg.Kind {
	case KindBase:
		return fmt.Sprintf("game v%d", pkg.Version)
	case KindUpdate:
		return fmt.Sprintf("update v%d", pkg.Version)
	case KindAddon:
		return fmt.Sprintf("DLC %s v%d", pkg.TitleIDText(), pkg.Version)
	}
	return fmt.Sprintf("%s v%d", pkg.KindName(), pkg.Version)
}
