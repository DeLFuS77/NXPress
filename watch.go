package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// how often a watched folder is looked at
const watchEvery = 30 * time.Second

// watchJobs does the action on the files given, then keeps looking at them: a new file is
// taken once it is whole (see folderWatch.look). It goes on until ctx ends, and gives how many
// files failed.
func watchJobs(ctx context.Context, o options, out output, every time.Duration) int {
	return watchFolder(ctx, o, out, every, runJobs)
}

// watchFolder is watchJobs with the work it does on the files given.
func watchFolder(ctx context.Context, o options, out output, every time.Duration, run func(context.Context, options, output) int) int {
	// every round of the watch claims its new names in one set: with -w, a round does not
	// replace the new file of one before
	ctx = withDestinations(ctx, nil)
	extensions := takes[o.action()]
	watch := folderWatch{seen: map[string]bool{}, looked: map[string]fileLook{}, failed: map[string]fileLook{}}
	// the files there at the start are done first, as they are
	files, _ := inputs(o.files, extensions)
	taken := map[string]fileLook{}
	for _, path := range files {
		watch.seen[watchKey(path)] = true
		if look, ok := lookAt(path); ok {
			taken[watchKey(path)] = look
		}
	}
	ends := &watchOutput{output: out}
	failed := run(ctx, o, ends)
	watch.ended(ends.take(), taken)
	out.say("watching for new files")
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return failed
		case <-ticker.C:
		}
		files, _ := inputs(o.files, extensions)
		ready, taken := watch.look(files)
		if len(ready) == 0 {
			continue
		}
		next := o
		next.files = ready
		failed += run(ctx, next, ends)
		watch.ended(ends.take(), taken)
	}
}

// folderWatch tells, look after look, which files of a watched folder are new and whole.
type folderWatch struct {
	// the files taken, and the ones there at the start
	seen map[string]bool
	// how the files not taken yet looked the last time
	looked map[string]fileLook
	// how the files whose work failed looked when they were taken: they are taken again once
	// they change, as a copy that goes on does
	failed map[string]fileLook
}

// fileLook is how a file looks from outside: its size and when it was last written. A file in
// parts adds up its parts.
type fileLook struct {
	size     int64
	modified int64
	parts    int
}

// look gives the files that are new and whole, with how they looked as they were taken. A file
// is whole once it has the same size and was last written at the same time in two looks, and
// no other program has it open to write (a copy sets the size of the file as it starts, and
// writes it until it ends).
func (w *folderWatch) look(files []string) (ready []string, taken map[string]fileLook) {
	taken = map[string]fileLook{}
	present := map[string]bool{}
	for _, path := range files {
		key := watchKey(path)
		present[key] = true
		if w.seen[key] {
			continue
		}
		look, ok := lookAt(path)
		if !ok {
			delete(w.looked, key)
			continue
		}
		if before, ok := w.failed[key]; ok {
			if before == look {
				continue
			}
			delete(w.failed, key)
		}
		if before, ok := w.looked[key]; ok && before == look && look.size > 0 && !partsBusy(path) {
			ready = append(ready, path)
			taken[key] = look
			w.seen[key] = true
			delete(w.looked, key)
			continue
		}
		w.looked[key] = look
	}
	// a file gone is looked at anew if it comes back
	for key := range w.looked {
		if !present[key] {
			delete(w.looked, key)
		}
	}
	for key := range w.failed {
		if !present[key] {
			delete(w.failed, key)
		}
	}
	return ready, taken
}

// ended takes in how the files taken ended: a file that failed is no longer seen, and is taken
// again once it changes. The files the work wrote are not new files to take, when it writes
// them in the folder watched.
func (w *folderWatch) ended(ends []watchEnd, taken map[string]fileLook) {
	for _, end := range ends {
		if end.written != "" {
			w.seen[watchKey(end.written)] = true
		}
		if !end.failed {
			continue
		}
		key := watchKey(end.path)
		look, ok := taken[key]
		if !ok {
			continue
		}
		delete(w.seen, key)
		w.failed[key] = look
	}
}

// watchKey is how a file is known among the files watched: on Windows, a name in other
// capitals is the same file.
func watchKey(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

// lookAt tells how a file looks; false when it, or one of its parts, cannot be looked at.
func lookAt(path string) (fileLook, bool) {
	var look fileLook
	for _, part := range watchedParts(path) {
		info, err := os.Stat(part)
		if err != nil || info.IsDir() {
			return fileLook{}, false
		}
		look.size += info.Size()
		look.modified = max(look.modified, info.ModTime().UnixNano())
		look.parts++
	}
	return look, look.parts > 0
}

// partsBusy tells whether another program still writes the file or one of its parts.
func partsBusy(path string) bool {
	for _, part := range watchedParts(path) {
		if fileBusy(part) {
			return true
		}
	}
	return false
}

// watchedParts are the files a file of the folder is made of: itself, or its parts, in a folder
// (00, 01...) or as name.ns0, name.ns1...
func watchedParts(path string) []string {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if _, ok := splitContainer(path); !ok {
		return []string{path}
	}
	parts := []string{}
	for n := 0; ; n++ {
		part := filepath.Join(path, fmt.Sprintf("%02d", n))
		if !info.IsDir() {
			part = strings.TrimSuffix(path, "0") + strconv.Itoa(n)
		}
		if _, err := os.Stat(part); err != nil {
			return parts
		}
		parts = append(parts, part)
	}
}

// watchOutput passes on what the work tells, and keeps how each file ended for the watch.
type watchOutput struct {
	output
	mutex sync.Mutex
	ends  []watchEnd
}

// watchEnd is how the work on a file ended: the file it wrote, or a failure.
type watchEnd struct {
	path    string
	written string
	failed  bool
}

func (w *watchOutput) done(path string, written string, note string, err error) {
	w.mutex.Lock()
	w.ends = append(w.ends, watchEnd{path: path, written: written, failed: err != nil && !isSkipped(err)})
	w.mutex.Unlock()
	w.output.done(path, written, note, err)
}

// take gives how the files ended since the last time.
func (w *watchOutput) take() []watchEnd {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	ends := w.ends
	w.ends = nil
	return ends
}

// waitWhilePaused and measured go on to the window, which pauses the work and shows a measure.
func (w *watchOutput) waitWhilePaused() {
	if pausing, ok := w.output.(interface{ waitWhilePaused() }); ok {
		pausing.waitWhilePaused()
	}
}

func (w *watchOutput) measured(report measureReport) {
	if window, ok := w.output.(measured); ok {
		window.measured(report)
	}
}
