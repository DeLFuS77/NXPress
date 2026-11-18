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
// until fewer are left. The command line has none: its work keeps what it was given.
type Live struct {
	mutex   sync.Mutex
	threads int
	multi   int
	// closed and made again at each change, so the work waiting for a free place looks again
	changed chan struct{}
}

// NewLive gives the threads and the files at once a work starts with.
func NewLive(threads int, multi int) *Live {
	l := &Live{changed: make(chan struct{})}
	l.threads, l.multi = liveValues(threads, multi)
	return l
}

// liveValues are threads and multi as a work takes them: one file at once at least, and the
// threads of each file left to choose (below 1) as many as the files at once leave each.
func liveValues(threads int, multi int) (int, int) {
	multi = max(1, multi)
	if threads < 1 {
		threads = DefaultThreads(multi)
	}
	return threads, multi
}

// Set changes the threads and the files at once, from the next file on.
func (l *Live) Set(threads int, multi int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.threads, l.multi = liveValues(threads, multi)
	close(l.changed)
	l.changed = make(chan struct{})
}

// Values are the threads of each file and the files at once set now.
func (l *Live) Values() (threads int, multi int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.threads, l.multi
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

// now gives o with the threads and the files at once set now, and what closes when they change
// (nil, which never closes, when they do not).
func (l *Live) now(o Options) (Options, <-chan struct{}) {
	if l == nil {
		return o, nil
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	o.Threads, o.Multi = l.threads, l.multi
	return o, l.changed
}

// eachFile does job on every file, as many at once as the work allows, each with the options it
// starts with: with Live (the window) the threads of each file and the files at once are read as
// each file starts, so a change applies from the next file. A file starts only while the work is
// not paused, and none once it is cancelled. It returns once every file started has ended.
func eachFile(ctx context.Context, o Options, files []string, out Output, job func(path string, o Options)) {
	live := liveOf(ctx)
	// the files that ended, told to the loop that starts them; a place for each, so a file that
	// ends never waits for it
	ended := make(chan struct{}, len(files))
	var wait sync.WaitGroup
	running := 0
	defer wait.Wait()
	for _, path := range files {
		for {
			waitWhilePaused(out)
			if ctx.Err() != nil {
				return
			}
			now, changed := live.now(o)
			if running < max(1, now.Multi) {
				running++
				wait.Add(1)
				go func(path string, o Options) {
					defer wait.Done()
					defer func() { ended <- struct{}{} }()
					job(path, o)
				}(path, now)
				break
			}
			select {
			case <-ended:
				running--
			case <-changed:
			case <-ctx.Done():
			}
		}
	}
}
