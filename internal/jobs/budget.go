package jobs

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// With a total of threads (--total-threads), NXPress shares them among the files at work by
// itself, instead of giving every file the same threads. A file takes its share as it starts and
// keeps it to its end: the zstd library cannot change the threads of a stream in the middle of
// it. The threads free then go to it and to the files that start with it, one at least each and
// the rest to the one that would end last (shareThreads), never more than its stream can keep
// busy (frameThreads); what it cannot use, and the threads of the files that end, go to the files
// that start next. A file starts only while a thread is free, so the threads at work never go
// above the total. When the total goes down while the work goes on, the files at work keep their
// threads and no file starts until fewer are at work. The free memory caps the total, as it caps
// the threads of each file without it (switchfs.LimitThreads).
//
// Each NCA is a zstd stream of its own, so a file that holds several (a game with its update and
// DLC, an XCI) could take its share again as each NCA starts, with the threads the files that
// ended gave back (renew). switchfs starts the encoder of every NCA of a file with the threads
// the file took, so for now a file keeps its share to its end.

// piece is what starts and takes threads: a file, or the next NCA of a file at work.
type piece struct {
	// the bytes it has left to do, which its share goes by
	bytes int64
	// the threads it can keep busy, 0 for no limit
	most int
}

// shareThreads shares free threads among the pieces that start now, the first one first: by the
// bytes each has to do, one at least and never more than it can keep busy, so what one cannot
// use goes to the others. With fewer threads than pieces, the first ones take one each and the
// others none. What none can use stays free.
//
// Each piece takes one, then each thread left goes to the piece with the most bytes for each
// thread it holds: the one that would end last, as the threads of a stream go about as fast
// each. The work ends when its last file does, and so each thread goes where it brings that end
// closest. One big file and six of an eighth of it on 21 threads take 9 and 2 each; shares cut
// by the bytes alone (12, and 1 or 2) leave three small files with one thread, which end last,
// about a tenth later.
func shareThreads(free int, pieces []piece) []int {
	shares := make([]int, len(pieces))
	if free < 1 || len(pieces) == 0 {
		return shares
	}
	if free < len(pieces) {
		for i := range free {
			shares[i] = 1
		}
		return shares
	}
	// the bytes in MiB, one at least, so the products below stay far from the limits of int64
	weights := make([]int64, len(pieces))
	for i, p := range pieces {
		weights[i] = max(1, p.bytes>>20)
		shares[i] = 1
	}
	for left := free - len(pieces); left > 0; left-- {
		best := -1
		for i, p := range pieces {
			if p.most > 0 && shares[i] >= p.most {
				continue
			}
			// weights[i] / shares[i] above the one of best, the first one on a tie
			if best < 0 || weights[i]*int64(shares[best]) > weights[best]*int64(shares[i]) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		shares[best]++
	}
	return shares
}

// filesAtOnce are the most files at work at once: -m, and with a total of threads and -m left to
// choose, one file for every three threads of it. Three threads a file keep the jobs of its
// stream busy, while the windows of 128 MiB each stream takes and the files read at once stay
// few; the bigger files get the bigger shares.
func filesAtOnce(o Options) int {
	if o.TotalThreads > 0 && o.Multi < 1 {
		return max(1, o.TotalThreads/3)
	}
	return max(1, o.Multi)
}

// FilesAtOnce are the most files of the work of o at work at once (see filesAtOnce), for what
// weighs them before it starts: the room their new files take.
func FilesAtOnce(o Options) int {
	return filesAtOnce(o)
}

// zstdJobSize is the data each thread of the zstd library in C takes at once on one stream, as
// switchfs sets it (nativeJobSize there): with the long window, 32 MiB above level 11 (since
// 1.7.16, so a big game keeps every thread busy to its end) and below it 8 times the chain table
// of the level, 2 MiB at least; with the window of the level, 4 times that window.
func zstdJobSize(level int, long bool) int64 {
	if long {
		switch {
		case level > 11:
			return 32 << 20
		case level >= 10:
			return 16 << 20
		case level == 9:
			return 8 << 20
		case level >= 7:
			return 4 << 20
		}
		return 2 << 20
	}
	windowLog := 23
	switch {
	case level <= 1:
		windowLog = 19
	case level == 2:
		windowLog = 20
	case level <= 8:
		windowLog = 21
	case level <= 16:
		windowLog = 22
	}
	return 1 << (windowLog + 2)
}

// frameThreads are the threads the zstd encoder of a file can keep busy, 0 for no limit. The zstd
// library in C gives each thread a job of zstdJobSize bytes of a stream, one stream for each NCA,
// so a file whose biggest NCA makes n jobs keeps n threads busy at most: a DLC of 50 MiB keeps 2
// at the high levels, a game of 4 GiB 128. An NSZ is read for the size it decompresses to. The
// files compressed in blocks (-B, and every XCI), the extra compression of the LZ4 blocks, which
// makes the stream bigger than the NCA, and the actions that do not compress have no limit.
func frameThreads(path string, o Options) int {
	action := o.Action()
	if (action != "compress" && action != "recompress") || o.Block || o.Archive || !switchfs.NativeZstd {
		return 0
	}
	var biggest int64
	switch strings.ToLower(filepath.Ext(path)) {
	case ".nsp":
		pfs, err := switchfs.ReadPfs0File(path)
		if err != nil {
			return 0
		}
		for _, file := range pfs.Files {
			biggest = max(biggest, int64(file.Size))
		}
	case ".nsz":
		size, err := switchfs.DecompressedSize(path)
		if err != nil {
			return 0
		}
		biggest = size
	default:
		return 0
	}
	job := zstdJobSize(o.Level, !o.ShortWindow)
	return int(max(1, (biggest+job-1)/job))
}

// memoryThreads are the most threads the free memory takes at a level, all files together, as
// switchfs.LimitThreads counts them; 0 for no limit. The tests stand in for it.
var memoryThreads = func(level int) int {
	const many = 1 << 16
	if fit := switchfs.LimitThreads(strconv.Itoa(level), many, 1); fit < many {
		return fit
	}
	return 0
}

// inputSize is the size of a file given, or of all the files in a folder given (a dump folder, a
// folder of parts); 0 when it cannot be read.
func inputSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}
	var size int64
	filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			if info, err := entry.Info(); err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}

// shareFile is a file of a work, waiting or at work.
type shareFile struct {
	path  string
	bytes int64
	// the threads its stream can keep busy (0 for no limit), -1 until they are read
	most int
	// the threads it holds while at work
	threads int
}

// sharing are the files of a work and the threads they hold: the files waiting, in the order they
// were given, and the files at work, with the threads in use. It gives each file the threads it
// starts with, and with a total of threads, its share of them.
type sharing struct {
	mutex sync.Mutex
	// the threads and the files at once the window changes, nil without a window
	live *Live
	// the options of the work, which live changes
	base    Options
	waiting []*shareFile
	// the same files by size, the biggest first, made once the biggest first are asked
	bySize  []*shareFile
	working int
	inUse   int
	// the most threads the free memory takes at the level of the work, as it starts; 0 for no
	// limit
	memory int
	// closed and made again when a file ends or gives back threads
	wake chan struct{}
	// the threads a file can keep busy (frameThreads); the tests stand in for it
	usable func(path string, o Options) int
}

// newSharing gives the files of a work, each with its size, and the threads the memory takes.
func newSharing(o Options, files []string, live *Live) *sharing {
	s := &sharing{live: live, base: o, memory: memoryThreads(o.Level), wake: make(chan struct{}), usable: frameThreads}
	for _, path := range files {
		s.waiting = append(s.waiting, &shareFile{path: path, bytes: inputSize(path), most: -1})
	}
	return s
}

// left is how many files have not started yet.
func (s *sharing) left() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.waiting)
}

// budget is the total of threads, capped by the memory.
func (s *sharing) budget(total int) int {
	if s.memory > 0 {
		return min(total, s.memory)
	}
	return total
}

// piece is a file waiting as the share of the threads sees it.
func (s *sharing) piece(f *shareFile, o Options) piece {
	if f.most < 0 {
		f.most = s.usable(f.path, o)
	}
	return piece{bytes: f.bytes, most: f.most}
}

// order are the files waiting in the order they start: as given, or the biggest first.
func (s *sharing) order(o Options) []*shareFile {
	if !o.BiggestFirst {
		return s.waiting
	}
	if s.bySize == nil {
		s.bySize = slices.Clone(s.waiting)
		slices.SortStableFunc(s.bySize, func(a, b *shareFile) int {
			switch {
			case a.bytes > b.bytes:
				return -1
			case a.bytes < b.bytes:
				return 1
			}
			return 0
		})
	}
	return s.bySize
}

// remove takes a file that starts out of the files waiting.
func (s *sharing) remove(f *shareFile) {
	started := func(w *shareFile) bool { return w == f }
	s.waiting = slices.DeleteFunc(s.waiting, started)
	if s.bySize != nil {
		s.bySize = slices.DeleteFunc(s.bySize, started)
	}
}

// share is the share of the free threads of the first piece, with the files waiting that start
// in the places free with it. A file past the free threads would take none, and leaves the
// share of the first as it is.
func (s *sharing) share(o Options, first piece, others []*shareFile, places int) int {
	free := s.budget(o.TotalThreads) - s.inUse
	pieces := []piece{first}
	for _, other := range others[:max(0, min(places, len(others), free-1))] {
		pieces = append(pieces, s.piece(other, o))
	}
	return shareThreads(free, pieces)[0]
}

// next takes the next file to start and the options it starts with, its threads among them; or
// nil, and what to wait on before looking again: a file that ends or gives back threads, and a
// change of the threads or the files at once.
func (s *sharing) next() (*shareFile, Options, <-chan struct{}, <-chan struct{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	o, changed := s.live.now(s.base)
	if len(s.waiting) == 0 || s.working >= filesAtOnce(o) {
		return nil, o, s.wake, changed
	}
	order := s.order(o)
	f := order[0]
	threads := o.Threads
	if o.TotalThreads > 0 {
		if s.budget(o.TotalThreads)-s.inUse < 1 {
			return nil, o, s.wake, changed
		}
		threads = s.share(o, s.piece(f, o), order[1:], filesAtOnce(o)-s.working-1)
	}
	s.remove(f)
	s.working++
	f.threads = threads
	s.inUse += threads
	o.Threads = threads
	return f, o, nil, nil
}

// end gives back the threads of a file that ended.
func (s *sharing) end(f *shareFile) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.working--
	s.inUse -= f.threads
	f.threads = 0
	s.signal()
}

// signal wakes whoever waits for threads given back.
func (s *sharing) signal() {
	close(s.wake)
	s.wake = make(chan struct{})
}

// renew is what a file at work does as it starts another NCA, with a total of threads: it gives
// back its threads and takes its share again of the ones free then, by the bytes it has left
// (this NCA and the ones after it), never more than the stream of this NCA keeps busy (most, 0
// for no limit). The threads the files that ended gave back go to it, and to the files waiting
// that start in the places free with it. Without a total the file keeps its threads. It waits
// while none is free (the total went down meanwhile), and gives 0 when ctx ends first.
func (s *sharing) renew(ctx context.Context, f *shareFile, bytes int64, most int) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	held := f.threads
	for {
		o, changed := s.live.now(s.base)
		if o.TotalThreads == 0 && f.threads > 0 {
			return f.threads
		}
		s.inUse -= f.threads
		f.threads = 0
		if o.TotalThreads == 0 {
			f.threads = o.Threads
		} else if s.budget(o.TotalThreads)-s.inUse >= 1 {
			f.threads = s.share(o, piece{bytes: bytes, most: most}, s.order(o), filesAtOnce(o)-s.working)
		}
		if f.threads > 0 {
			s.inUse += f.threads
			if f.threads < held {
				s.signal()
			}
			return f.threads
		}
		// what it gave back may let others start or take theirs
		s.signal()
		wake := s.wake
		s.mutex.Unlock()
		select {
		case <-wake:
		case <-changed:
		case <-ctx.Done():
		}
		s.mutex.Lock()
		if ctx.Err() != nil {
			return 0
		}
	}
}
