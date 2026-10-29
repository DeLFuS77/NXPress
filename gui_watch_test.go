package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
)

// watchRecorder is the window of a watch under test: it keeps the files that ended.
type watchRecorder struct {
	mutex    sync.Mutex
	ended    []string
	watching bool
}

func (r *watchRecorder) progress(string, string) func(int64, int64) { return func(int64, int64) {} }

func (r *watchRecorder) done(path string, output string, note string, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.ended = append(r.ended, path)
}

func (r *watchRecorder) say(format string, args ...any) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.watching = r.watching || format == "watching for new files"
}

// startWatch watches a folder until the test ends, once the files of the start are done.
func startWatch(t *testing.T, o options, runs *watchRuns) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	recorder := &watchRecorder{}
	go func() {
		watchFolder(ctx, o, recorder, 10*time.Millisecond, runs.run)
		close(stopped)
	}()
	eventually(t, "the watch did not start", func() bool {
		recorder.mutex.Lock()
		defer recorder.mutex.Unlock()
		return recorder.watching
	})
}

// watchRuns is the work of a watch under test: it keeps the files it was given, and ends each
// one as the test says.
type watchRuns struct {
	mutex sync.Mutex
	given []string
	end   func(path string) (string, error)
}

func (w *watchRuns) run(ctx context.Context, o options, out output) int {
	files, _ := inputs(o.files, takes[o.action()])
	failed := 0
	for _, path := range files {
		w.mutex.Lock()
		w.given = append(w.given, filepath.Base(path))
		end := w.end
		w.mutex.Unlock()
		written, err := "", error(nil)
		if end != nil {
			written, err = end(path)
		}
		out.done(path, written, "", err)
		if err != nil {
			failed++
		}
	}
	return failed
}

func (w *watchRuns) times(name string) int {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	n := 0
	for _, given := range w.given {
		if given == name {
			n++
		}
	}
	return n
}

// eventually waits for what is wanted, a few seconds at most.
func eventually(t *testing.T, what string, wanted func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !wanted(); {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A file is taken once its size and the time it was last written stay the same from one look to
// the next, and not while it is empty.
func TestWatchTakesWholeFiles(t *testing.T) {
	folder := t.TempDir()
	whole := filepath.Join(folder, "whole.nsp")
	growing := filepath.Join(folder, "growing.nsp")
	touched := filepath.Join(folder, "touched.nsp")
	empty := filepath.Join(folder, "empty.nsp")
	writeFile(t, whole, 100)
	writeFile(t, growing, 10)
	writeFile(t, touched, 100)
	writeFile(t, empty, 0)
	written := time.Now().Add(-time.Hour)
	os.Chtimes(touched, written, written)

	watch := folderWatch{seen: map[string]bool{}, looked: map[string]fileLook{}, failed: map[string]fileLook{}}
	look := func() []string {
		files, _ := inputs([]string{folder}, takes["compress"])
		ready, _ := watch.look(files)
		names := []string{}
		for _, path := range ready {
			names = append(names, filepath.Base(path))
		}
		slices.Sort(names)
		return names
	}
	if ready := look(); len(ready) > 0 {
		t.Fatalf("taken at the first look: %v", ready)
	}
	// it grows, and the other is written again at the same size
	writeFile(t, growing, 20)
	os.Chtimes(touched, written.Add(time.Minute), written.Add(time.Minute))
	if ready := look(); !slices.Equal(ready, []string{"whole.nsp"}) {
		t.Fatalf("second look: %v, want whole.nsp alone", ready)
	}
	if ready := look(); !slices.Equal(ready, []string{"growing.nsp", "touched.nsp"}) {
		t.Fatalf("third look: %v, want the files that stayed the same", ready)
	}
	if ready := look(); len(ready) > 0 {
		t.Fatalf("taken again, or the empty file taken: %v", ready)
	}
}

// A copy sets the size of the file as it starts: a file of its final size still open to write is
// not taken until it is closed. Only Windows tells a file open to write.
func TestWatchWaitsForAFileOpenToWrite(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows tells a file open to write")
	}
	folder := t.TempDir()
	path := filepath.Join(folder, "copying.nsp")
	runs := &watchRuns{}
	startWatch(t, options{compress: true, files: []string{folder}}, runs)
	copying, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer copying.Close()
	if err := copying.Truncate(1 << 20); err != nil {
		t.Fatal(err)
	}
	if !fileBusy(path) {
		t.Fatal("a file open to write is not told busy")
	}

	time.Sleep(300 * time.Millisecond)
	if runs.times("copying.nsp") > 0 {
		t.Fatal("a file still open to write was taken")
	}
	copying.Close()
	if fileBusy(path) {
		t.Fatal("a closed file is told busy")
	}
	eventually(t, "the file was not taken once closed", func() bool { return runs.times("copying.nsp") == 1 })
}

// A file whose work failed is not given up: it is taken again once it changes, as a copy taken
// too soon goes on, and not while it stays the same.
func TestWatchRetriesAFailedFileOnceItChanges(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "game.nsp")
	failures := 1
	runs := &watchRuns{end: func(string) (string, error) {
		if failures > 0 {
			failures--
			return "", errors.New("read 0 of 3584 bytes: EOF")
		}
		return "", nil
	}}
	startWatch(t, options{compress: true, files: []string{folder}}, runs)
	writeFile(t, path, 100)
	eventually(t, "the new file was not taken", func() bool { return runs.times("game.nsp") == 1 })
	time.Sleep(300 * time.Millisecond)
	if n := runs.times("game.nsp"); n != 1 {
		t.Fatalf("a failed file that stayed the same was taken %d times", n)
	}
	writeFile(t, path, 200)
	eventually(t, "the failed file was not taken again once it changed", func() bool { return runs.times("game.nsp") == 2 })
	time.Sleep(300 * time.Millisecond)
	if n := runs.times("game.nsp"); n != 2 {
		t.Fatalf("a file done was taken %d times", n)
	}
}

// The files there at the start are done at once; one that fails is taken again once it changes.
func TestWatchRetriesAFailedFileOfTheStart(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "game.nsp")
	writeFile(t, path, 100)
	failures := 1
	runs := &watchRuns{end: func(string) (string, error) {
		if failures > 0 {
			failures--
			return "", errors.New("unexpected EOF")
		}
		return "", nil
	}}
	startWatch(t, options{compress: true, files: []string{folder}}, runs)
	eventually(t, "the file of the start was not done", func() bool { return runs.times("game.nsp") == 1 })
	time.Sleep(200 * time.Millisecond)
	if n := runs.times("game.nsp"); n != 1 {
		t.Fatalf("a failed file that stayed the same was taken %d times", n)
	}
	writeFile(t, path, 300)
	eventually(t, "the failed file was not taken again once it changed", func() bool { return runs.times("game.nsp") == 2 })
}

// The watch passes the pause of the window on to the work.
func TestWatchOutputPassesThePauseOn(t *testing.T) {
	app := &App{}
	app.paused.Store(true)
	job, cancel := context.WithCancel(context.Background())
	var out output = &watchOutput{output: &window{job: job, app: app}}
	pausing, ok := out.(interface{ waitWhilePaused() })
	if !ok {
		t.Fatal("the watch hides the pause of the window")
	}
	released := make(chan struct{})
	go func() {
		pausing.waitWhilePaused()
		close(released)
	}()
	select {
	case <-released:
		t.Fatal("the work went on while paused")
	case <-time.After(300 * time.Millisecond):
	}
	cancel()
	<-released
	// the console has no pause: nothing waits
	(&watchOutput{output: &watchRecorder{}}).waitWhilePaused()
}

// The files the work writes in the folder watched are not taken as new files: recompressed into
// a folder within it, each new file would be recompressed once more.
func TestWatchLeavesTheFilesItWrites(t *testing.T) {
	folder := t.TempDir()
	recompressed := filepath.Join(folder, "Recompressed")
	runs := &watchRuns{}
	runs.end = func(path string) (string, error) {
		os.MkdirAll(recompressed, 0o755)
		written := filepath.Join(recompressed, filepath.Base(path))
		return written, os.WriteFile(written, make([]byte, 50), 0o644)
	}
	startWatch(t, options{recompress: true, files: []string{folder}}, runs)
	writeFile(t, filepath.Join(folder, "game.nsz"), 100)
	eventually(t, "the new file was not taken", func() bool { return runs.times("game.nsz") == 1 })
	time.Sleep(300 * time.Millisecond)
	if n := runs.times("game.nsz"); n != 1 {
		t.Fatalf("the file written by the work was taken: game.nsz given %d times", n)
	}
}
