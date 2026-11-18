package jobs

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DeLFuS77/NXPress/internal/testkit"
)

// heldJobs is a job of eachFile that holds each file until it is let go, and keeps how many are
// at work at once and the options each started with.
type heldJobs struct {
	mutex   sync.Mutex
	at      int
	most    int
	started []Options
	release chan struct{}
}

func newHeldJobs() *heldJobs {
	return &heldJobs{release: make(chan struct{})}
}

func (h *heldJobs) job(path string, o Options) {
	h.mutex.Lock()
	h.at++
	h.most = max(h.most, h.at)
	h.started = append(h.started, o)
	h.mutex.Unlock()
	<-h.release
	h.mutex.Lock()
	h.at--
	h.mutex.Unlock()
}

// running is how many files are at work, and how many started.
func (h *heldJobs) running() (int, int) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return h.at, len(h.started)
}

// waitFor waits until at files are at work and started have started.
func (h *heldJobs) waitFor(t *testing.T, at int, started int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if a, s := h.running(); a == at && s == started {
			return
		}
		time.Sleep(time.Millisecond)
	}
	a, s := h.running()
	t.Fatalf("%d at work and %d started, want %d and %d", a, s, at, started)
}

// stays checks that no other file starts for a while.
func (h *heldJobs) stays(t *testing.T, at int, started int) {
	t.Helper()
	time.Sleep(50 * time.Millisecond)
	if a, s := h.running(); a != at || s != started {
		t.Fatalf("%d at work and %d started, want still %d and %d", a, s, at, started)
	}
}

func files(n int) []string {
	paths := []string{}
	for i := range n {
		paths = append(paths, fmt.Sprintf("file%d.nsp", i))
	}
	return paths
}

// The files at once grow while the work goes on, and shrink: the files at work end as they are,
// and no new one starts until fewer than the new number are left.
func TestFilesAtOnceChangeWhileTheWorkGoesOn(t *testing.T) {
	live := NewLive(4, 1)
	h := newHeldJobs()
	done := make(chan struct{})
	go func() {
		eachFile(WithLive(context.Background(), live), Options{Threads: 4, Multi: 1}, files(8), testkit.Quiet{}, h.job)
		close(done)
	}()
	h.waitFor(t, 1, 1)
	h.stays(t, 1, 1)
	// up to three at once: two more start at once, while the first is still at work
	live.Set(4, 3)
	h.waitFor(t, 3, 3)
	h.stays(t, 3, 3)
	// down to one: the three at work go on, and none starts until only one would be at work
	live.Set(4, 1)
	h.stays(t, 3, 3)
	h.release <- struct{}{}
	h.waitFor(t, 2, 3)
	h.stays(t, 2, 3)
	h.release <- struct{}{}
	h.waitFor(t, 1, 3)
	h.stays(t, 1, 3)
	h.release <- struct{}{}
	h.waitFor(t, 1, 4)
	close(h.release)
	<-done
	if _, started := h.running(); started != 8 {
		t.Fatalf("%d files started, want 8", started)
	}
}

// Each file takes the threads set as it starts and keeps them: the one at work is not changed,
// the next one takes the new ones.
func TestThreadsAreReadAsEachFileStarts(t *testing.T) {
	live := NewLive(7, 1)
	h := newHeldJobs()
	done := make(chan struct{})
	go func() {
		eachFile(WithLive(context.Background(), live), Options{Threads: 7, Multi: 1, Level: 3}, files(3), testkit.Quiet{}, h.job)
		close(done)
	}()
	h.waitFor(t, 1, 1)
	live.Set(2, 1)
	h.release <- struct{}{}
	h.waitFor(t, 1, 2)
	close(h.release)
	<-done
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if got := [3]int{h.started[0].Threads, h.started[1].Threads, h.started[2].Threads}; got != [3]int{7, 2, 2} {
		t.Fatalf("threads of each file %v, want 7 then 2", got)
	}
	if h.started[0].Level != 3 {
		t.Fatal("the other options were not given to the file")
	}
}

// Without Live (the command line) the work keeps the files at once it was given.
func TestFilesAtOnceWithoutLive(t *testing.T) {
	h := newHeldJobs()
	done := make(chan struct{})
	go func() {
		eachFile(context.Background(), Options{Threads: 2, Multi: 2}, files(5), testkit.Quiet{}, h.job)
		close(done)
	}()
	h.waitFor(t, 2, 2)
	h.stays(t, 2, 2)
	close(h.release)
	<-done
	if h.most != 2 {
		t.Fatalf("%d files at once, want 2", h.most)
	}
}

// pausedOut holds every file that would start while the work is paused.
type pausedOut struct {
	testkit.Quiet
	paused atomic.Bool
}

func (p *pausedOut) WaitWhilePaused() {
	for p.paused.Load() {
		time.Sleep(time.Millisecond)
	}
}

// A paused work starts no file, and a cancelled one none more; it returns once the files at
// work end.
func TestNoFileStartsPausedOrCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	live := NewLive(1, 1)
	out := &pausedOut{}
	h := newHeldJobs()
	done := make(chan struct{})
	go func() {
		eachFile(WithLive(ctx, live), Options{Threads: 1, Multi: 1}, files(4), out, h.job)
		close(done)
	}()
	h.waitFor(t, 1, 1)
	out.paused.Store(true)
	live.Set(1, 4)
	h.stays(t, 1, 1)
	out.paused.Store(false)
	h.waitFor(t, 4, 4)
	close(h.release)
	<-done

	h = newHeldJobs()
	done = make(chan struct{})
	go func() {
		eachFile(WithLive(ctx, NewLive(1, 1)), Options{Threads: 1, Multi: 1}, files(4), testkit.Quiet{}, h.job)
		close(done)
	}()
	h.waitFor(t, 1, 1)
	cancel()
	select {
	case <-done:
		t.Fatal("the work ended before the file at work did")
	case <-time.After(50 * time.Millisecond):
	}
	close(h.release)
	<-done
	if _, started := h.running(); started != 1 {
		t.Fatalf("%d files started after the cancel, want none", started-1)
	}
}

// Threads left to choose are the share of each file among the files at once, and there is one
// file at once at least.
func TestLiveValues(t *testing.T) {
	live := NewLive(0, 0)
	if threads, multi := live.Values(); threads != DefaultThreads(1) || multi != 1 {
		t.Fatalf("threads %d, files at once %d", threads, multi)
	}
	live.Set(0, 2)
	if threads, multi := live.Values(); threads != DefaultThreads(2) || multi != 2 {
		t.Fatalf("threads %d, files at once %d", threads, multi)
	}
	live.Set(3, 5)
	if threads, multi := live.Values(); threads != 3 || multi != 5 {
		t.Fatalf("threads %d, files at once %d", threads, multi)
	}
}

// A compression whose files at once change while it goes on does every file, and checks each.
func TestCompressionWithFilesAtOnceChanged(t *testing.T) {
	testkit.Isolate(t)
	base, update, dlc := testkit.Game(t, t.TempDir())
	live := NewLive(1, 1)
	out := &changeOnProgress{live: live}
	o := Options{Compress: true, Level: 3, Threads: 1, Multi: 1, Files: []string{base, update, dlc}}
	if failed := RunJobs(WithLive(context.Background(), live), o, out); failed != 0 {
		t.Fatalf("%d failed: %v", failed, out.Results.Results)
	}
	for _, path := range []string{base, update, dlc} {
		if err := out.Results.Results[path].Err; err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if !testkit.Exists(path[:len(path)-len(".nsp")] + ".nsz") {
			t.Errorf("%s was not compressed", path)
		}
	}
}

// changeOnProgress raises the files at once and the threads as the first file tells its progress.
type changeOnProgress struct {
	testkit.Results
	live *Live
	once sync.Once
}

func (c *changeOnProgress) Progress(string, string) func(int64, int64) {
	return func(int64, int64) { c.once.Do(func() { c.live.Set(2, 3) }) }
}
