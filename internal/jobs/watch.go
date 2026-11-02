package jobs

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

	"github.com/DeLFuS77/NXPress/internal/system"
)

// how often a watched folder is looked at
const WatchEvery = 30 * time.Second

// WatchJobs does the action on the files given, then keeps looking at them: a new file is
// taken once it is whole (see folderWatch.look). It goes on until ctx ends, and gives how many
// files failed.
func WatchJobs(ctx context.Context, o Options, out Output, every time.Duration) int {
	return WatchFolder(ctx, o, out, every, RunJobs)
}

// WatchFolder is watchJobs with the work it does on the files given.
func WatchFolder(ctx context.Context, o Options, out Output, every time.Duration, run func(context.Context, Options, Output) int) int {
	// every round of the watch claims its new names in one set: with -w, a round does not
	// replace the new file of one before
	ctx = withDestinations(ctx, nil)
	extensions := Takes[o.Action()]
	watch := FolderWatch{Seen: map[string]bool{}, Looked: map[string]FileLook{}, Failed: map[string]FileLook{}}
	// the files there at the start are done first, as they are
	files, _ := Inputs(o.Files, extensions)
	taken := map[string]FileLook{}
	for _, path := range files {
		watch.Seen[watchKey(path)] = true
		if look, ok := lookAt(path); ok {
			taken[watchKey(path)] = look
		}
	}
	ends := &WatchOutput{Output: out}
	failed := run(ctx, o, ends)
	watch.ended(ends.Take(), taken)
	out.Say("watching for new files")
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return failed
		case <-ticker.C:
		}
		files, _ := Inputs(o.Files, extensions)
		ready, taken := watch.Look(files)
		if len(ready) == 0 {
			continue
		}
		next := o
		next.Files = ready
		failed += run(ctx, next, ends)
		watch.ended(ends.Take(), taken)
	}
}

// FolderWatch tells, look after look, which files of a watched folder are new and whole.
type FolderWatch struct {
	// the files taken, and the ones there at the start
	Seen map[string]bool
	// how the files not taken yet looked the last time
	Looked map[string]FileLook
	// how the files whose work failed looked when they were taken: they are taken again once
	// they change, as a copy that goes on does
	Failed map[string]FileLook
}

// FileLook is how a file looks from outside: its size and when it was last written. A file in
// parts adds up its parts.
type FileLook struct {
	size     int64
	modified int64
	parts    int
}

// Look gives the files that are new and whole, with how they looked as they were taken. A file
// is whole once it has the same size and was last written at the same time in two looks, and
// no other program has it open to write (a copy sets the size of the file as it starts, and
// writes it until it ends).
func (w *FolderWatch) Look(files []string) (ready []string, taken map[string]FileLook) {
	taken = map[string]FileLook{}
	present := map[string]bool{}
	for _, path := range files {
		key := watchKey(path)
		present[key] = true
		if w.Seen[key] {
			continue
		}
		look, ok := lookAt(path)
		if !ok {
			delete(w.Looked, key)
			continue
		}
		if before, ok := w.Failed[key]; ok {
			if before == look {
				continue
			}
			delete(w.Failed, key)
		}
		if before, ok := w.Looked[key]; ok && before == look && look.size > 0 && !partsBusy(path) {
			ready = append(ready, path)
			taken[key] = look
			w.Seen[key] = true
			delete(w.Looked, key)
			continue
		}
		w.Looked[key] = look
	}
	// a file gone is looked at anew if it comes back
	for key := range w.Looked {
		if !present[key] {
			delete(w.Looked, key)
		}
	}
	for key := range w.Failed {
		if !present[key] {
			delete(w.Failed, key)
		}
	}
	return ready, taken
}

// ended takes in how the files taken ended: a file that failed is no longer seen, and is taken
// again once it changes. The files the work wrote are not new files to take, when it writes
// them in the folder watched.
func (w *FolderWatch) ended(ends []watchEnd, taken map[string]FileLook) {
	for _, end := range ends {
		if end.written != "" {
			w.Seen[watchKey(end.written)] = true
		}
		if !end.failed {
			continue
		}
		key := watchKey(end.path)
		look, ok := taken[key]
		if !ok {
			continue
		}
		delete(w.Seen, key)
		w.Failed[key] = look
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
func lookAt(path string) (FileLook, bool) {
	var look FileLook
	for _, part := range watchedParts(path) {
		info, err := os.Stat(part)
		if err != nil || info.IsDir() {
			return FileLook{}, false
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
		if system.FileBusy(part) {
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
	if _, ok := SplitContainer(path); !ok {
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

// WatchOutput passes on what the work tells, and keeps how each file ended for the watch.
type WatchOutput struct {
	Output
	mutex sync.Mutex
	ends  []watchEnd
}

// watchEnd is how the work on a file ended: the file it wrote, or a failure.
type watchEnd struct {
	path    string
	written string
	failed  bool
}

func (w *WatchOutput) Done(path string, written string, note string, err error) {
	w.mutex.Lock()
	w.ends = append(w.ends, watchEnd{path: path, written: written, failed: isFailure(err)})
	w.mutex.Unlock()
	w.Output.Done(path, written, note, err)
}

// Take gives how the files ended since the last time.
func (w *WatchOutput) Take() []watchEnd {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	ends := w.ends
	w.ends = nil
	return ends
}

// WaitWhilePaused and measured go on to the window, which pauses the work and shows a measure.
func (w *WatchOutput) WaitWhilePaused() {
	if pausing, ok := w.Output.(interface{ WaitWhilePaused() }); ok {
		pausing.WaitWhilePaused()
	}
}

func (w *WatchOutput) measured(report MeasureReport) {
	if window, ok := w.Output.(measured); ok {
		window.measured(report)
	}
}
