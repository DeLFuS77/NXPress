package jobs

import (
	"context"
	"sync"
)

// Live are the threads of each file and the files at once of a work that the window changes
// while it goes on. A file takes the threads set as it starts and keeps them until it is done:
// the encoder of a file is started with its threads, and changing them halfway would leave the
// file to start again. The files at once grow or shrink between files: more start at once when
// they go up, and when they go down the files at work end as they are and no new one starts
// until fewer are left. With a total of threads, NXPress shares them among the files itself
// (see sharing): the total and the order of the files waiting change the same way, from the next
// file on. The command line has none: its work keeps what it was given.
type Live struct {
	mutex   sync.Mutex
	threads int
	multi   int
	// the total threads of the work, 0 when each file takes threads, and whether the files
	// waiting start biggest first
	total        int
	biggestFirst bool
	// closed and made again at each change, so the work waiting for a free place looks again
	changed chan struct{}
}

// NewLive gives the threads and the files at once a work starts with.
func NewLive(threads int, multi int) *Live {
	return &Live{threads: threads, multi: multi, changed: make(chan struct{})}
}

// NewLiveFor gives what the work of o starts with: its threads, files at once, total of threads
// and order.
func NewLiveFor(o Options) *Live {
	l := NewLive(o.Threads, o.Multi)
	l.total, l.biggestFirst = max(0, o.TotalThreads), o.BiggestFirst
	return l
}

// liveValues are threads and multi as a work takes them: one file at once at least (with a total
// of threads, 0 leaves them to the total), and the threads of each file left to choose (below
// 1) as many as the files at once leave each.
func liveValues(threads int, multi int, total int) (int, int) {
	if total > 0 && multi < 1 {
		multi = 0
	} else {
		multi = max(1, multi)
	}
	if threads < 1 {
		threads = DefaultThreads(multi)
	}
	return threads, multi
}

// Set changes the threads and the files at once, from the next file on.
func (l *Live) Set(threads int, multi int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.threads, l.multi = threads, multi
	l.change()
}

// SetTotal changes the total of threads shared among the files (0: each file takes the threads
// set) and whether the files waiting start biggest first, from the next file on.
func (l *Live) SetTotal(total int, biggestFirst bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.total, l.biggestFirst = max(0, total), biggestFirst
	l.change()
}

// change tells the work waiting for a free place to look again.
func (l *Live) change() {
	close(l.changed)
	l.changed = make(chan struct{})
}

// Values are the threads of each file and the files at once set now.
func (l *Live) Values() (threads int, multi int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return liveValues(l.threads, l.multi, l.total)
}

// Total is the total of threads set now (0 for none) and whether the biggest files start first.
func (l *Live) Total() (int, bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.total, l.biggestFirst
}

type liveKey struct{}

// WithLive gives ctx the threads and the files at once the window changes while the work goes on.
func WithLive(ctx context.Context, l *Live) context.Context {
	return context.WithValue(ctx, liveKey{}, l)
}

// liveOf gives the threads and the files at once of the work, nil when they do not change.
func liveOf(ctx context.Context) *Live {
	l, _ := ctx.Value(liveKey{}).(*Live)
	return l
}

// now gives o with the threads, the files at once and the total set now, and what closes when
// they change (nil, which never closes, when they do not).
func (l *Live) now(o Options) (Options, <-chan struct{}) {
	if l == nil {
		return o, nil
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	o.TotalThreads, o.BiggestFirst = l.total, l.biggestFirst
	o.Threads, o.Multi = liveValues(l.threads, l.multi, l.total)
	return o, l.changed
}

// ThreadsTold is told the threads a file takes of a total of threads as it starts, and 0 as it
// ends and gives them back: the window shows them on the file, and the threads in use. Its method
// is exported, as the window is in another package.
type ThreadsTold interface {
	FileThreads(path string, threads int)
}

// tellThreads tells out the threads a file holds now, when it is told them.
func tellThreads(out Output, path string, threads int) {
	if told, ok := out.(ThreadsTold); ok {
		told.FileThreads(path, threads)
	}
}

// eachFile does job on every file, as many at once as the work allows, each with the options it
// starts with: with Live (the window) the threads of each file, the files at once and the total
// of threads are read as each file starts, so a change applies from the next file. With a total
// of threads, each file starts with its share of them (see sharing). A file starts only while
// the work is not paused, and none once it is cancelled. It returns once every file started has
// ended.
func eachFile(ctx context.Context, o Options, files []string, out Output, job func(path string, o Options)) {
	s := newSharing(o, files, liveOf(ctx))
	if now, _ := s.live.now(o); now.TotalThreads > 0 && s.memory > 0 && s.memory < now.TotalThreads {
		out.Say("%d threads in all at level %d: the free memory takes no more (%d asked)", s.memory, o.Level, now.TotalThreads)
	}
	var wait sync.WaitGroup
	defer wait.Wait()
	for s.left() > 0 {
		waitWhilePaused(out)
		if ctx.Err() != nil {
			return
		}
		f, now, wake, changed := s.next()
		if f != nil {
			wait.Add(1)
			shared := now.TotalThreads > 0
			if shared {
				tellThreads(out, f.path, now.Threads)
			}
			go func() {
				defer wait.Done()
				defer s.end(f)
				// told before they are given back, so a file that starts with them is told after
				if shared {
					defer tellThreads(out, f.path, 0)
				}
				job(f.path, now)
			}()
			continue
		}
		select {
		case <-wake:
		case <-changed:
		case <-ctx.Done():
		}
	}
}
