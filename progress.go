package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"
)

// output takes the progress and the results of the files, from several at once: the console,
// or the window.
type output interface {
	// progress gives the function a step calls with its bytes done and total, for one file
	progress(path string, step string) func(done int64, total int64)
	// done tells how a file ended: its new file, a note, or what went wrong
	done(path string, output string, note string, err error)
	// say tells something for people
	say(format string, args ...any)
}

// reporter prints the progress and the results of the files: as lines for people, as
// "<percentage>% <step>" (--minimal-output) or as one JSON object per line (--machine-readable)
// for other programs.
type reporter struct {
	mutex   sync.Mutex
	out     io.Writer
	minimal bool
	machine bool
}

// how often the progress of a file is printed
const progressEvery = 2 * time.Second

// progress gives the function a step calls with its bytes done and total, for one file.
func (r *reporter) progress(path string, step string) func(done int64, total int64) {
	var last time.Time
	lastPercent := -1
	return func(done int64, total int64) {
		percent := 0
		if total > 0 {
			percent = int(done * 100 / total)
		}
		now := time.Now()
		if percent == lastPercent || percent < 100 && now.Sub(last) < progressEvery {
			return
		}
		last, lastPercent = now, percent
		r.mutex.Lock()
		defer r.mutex.Unlock()
		switch {
		case r.machine:
			r.json(map[string]any{"file": path, "step": step, "done": done, "total": total, "percent": percent})
		case r.minimal:
			fmt.Fprintf(r.out, "%d%% %s\n", percent, step)
		default:
			fmt.Fprintf(r.out, "[%3d%%] %s %s\n", percent, step, filepath.Base(path))
		}
	}
}

// done tells how a file ended: its new file, or what went wrong.
func (r *reporter) done(path string, output string, note string, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.machine {
		result := map[string]any{"file": path, "result": "ok"}
		if output != "" {
			result["output"] = output
		}
		if note != "" {
			result["note"] = note
		}
		if err != nil {
			result["result"], result["error"] = "error", err.Error()
		}
		r.json(result)
		return
	}
	switch {
	case err != nil:
		fmt.Fprintf(r.out, "FAILED %s: %v\n", filepath.Base(path), err)
	case output != "" && note != "":
		fmt.Fprintf(r.out, "OK %s -> %s (%s)\n", filepath.Base(path), filepath.Base(output), note)
	case output != "":
		fmt.Fprintf(r.out, "OK %s -> %s\n", filepath.Base(path), filepath.Base(output))
	case note != "":
		fmt.Fprintf(r.out, "OK %s (%s)\n", filepath.Base(path), note)
	default:
		fmt.Fprintf(r.out, "OK %s\n", filepath.Base(path))
	}
}

// say prints a line for people; the other outputs leave it out.
func (r *reporter) say(format string, args ...any) {
	if r.minimal || r.machine {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	fmt.Fprintf(r.out, format+"\n", args...)
}

func (r *reporter) json(value map[string]any) {
	data, _ := json.Marshal(value)
	r.out.Write(append(data, '\n'))
}
