package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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

// reporter prints the progress and the results of the files: as lines for people, or for other
// programs as nsz prints them. With --minimal-output the progress is one line written over,
// "<percentage>% <step>", and "100% Done" ends it; with --machine-readable every line is a JSON
// object whose "type" is progress, complete, error, info, warning, heartbeat or summary.
type reporter struct {
	mutex   sync.Mutex
	out     io.Writer
	minimal bool
	machine bool
	// the action, which gives the code of an error as nsz numbers them
	action string
	// the files that failed, for the summary of --machine-readable
	failures []jsonObject
	// --minimal-output: the length of the line of progress, and the frame of its spinner
	line int
	spin int
}

// how often the progress of a file is printed, and the line of --minimal-output written over
const (
	progressEvery = 2 * time.Second
	minimalEvery  = 250 * time.Millisecond
)

// how often --machine-readable tells it is still at work, as nsz: a program running it tells a
// long step from a hang
const heartbeatEvery = 5 * time.Second

// the codes of the errors of --machine-readable, as nsz numbers them
const (
	codeCompress    = 104
	codeDecompress  = 105
	codeVerify      = 106
	codeOutputModes = 107
)

// the frames of the spinner of --minimal-output, as nsz turns it
var spinner = []string{"|", "/", "-", `\`}

// progress gives the function a step calls with its bytes done and total, for one file.
func (r *reporter) progress(path string, step string) func(done int64, total int64) {
	var last time.Time
	lastPercent := -1
	every := progressEvery
	if r.minimal {
		every = minimalEvery
	}
	return func(done int64, total int64) {
		percent := 0
		if total > 0 {
			percent = int(done * 100 / total)
		}
		now := time.Now()
		// the end is always told; a line written over turns its spinner even at the same percentage
		end := percent >= 100 && lastPercent < 100
		if !end && (now.Sub(last) < every || percent == lastPercent && !r.minimal) {
			return
		}
		last, lastPercent = now, percent
		r.mutex.Lock()
		defer r.mutex.Unlock()
		switch {
		case r.machine:
			exact := 0.0
			if total > 0 {
				exact = math.Round(float64(done)*1000/float64(total)) / 10
			}
			r.json(jsonObject{"type", "progress", "step", step, "file", path, "percent", exact,
				"currentRead", done, "totalRead", total, "done", done, "total", total})
		case r.minimal:
			r.overwrite(fmt.Sprintf("%s %d%% %s", spinner[r.spin%len(spinner)], min(percent, 100), step))
			r.spin++
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
		result := jsonObject{"type", "complete", "file", path, "result", "ok"}
		switch {
		case isSkipped(err):
			result = jsonObject{"type", "complete", "file", path, "result", "skipped"}
			note = err.Error()
		case err != nil:
			result = jsonObject{"type", "error", "code", r.errorCode(), "message", filepath.Base(path) + ": " + err.Error(),
				"file", path, "result", "error", "error", err.Error()}
			r.failures = append(r.failures, jsonObject{"filename", path, "message", err.Error()})
		}
		if output != "" {
			result = append(result, "output", output)
		}
		if note != "" {
			result = append(result, "note", note)
		}
		r.json(result)
		return
	}
	r.endLine()
	switch {
	case isSkipped(err):
		fmt.Fprintf(r.out, "SKIPPED %s (%v)\n", filepath.Base(path), err)
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

// say prints a line for people: the titles of -i, the levels of --levels, a measure. Other
// programs get it too, as {"type": "info"}.
func (r *reporter) say(format string, args ...any) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	text := fmt.Sprintf(format, args...)
	if r.machine {
		r.json(jsonObject{"type", "info", "message", text})
		return
	}
	r.endLine()
	fmt.Fprintln(r.out, text)
}

// note tells what is worth knowing before the work: "note: ..." for people, and
// {"type": "warning"} for other programs.
func (r *reporter) note(format string, args ...any) {
	if !r.machine {
		r.say("note: "+format, args...)
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.json(jsonObject{"type", "warning", "message", fmt.Sprintf(format, args...)})
}

// finish ends the output once every file is done: the summary of --machine-readable, as nsz
// gives it, or "100% Done" after the progress of --minimal-output.
func (r *reporter) finish() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	switch {
	case r.machine:
		failures := r.failures
		if failures == nil {
			failures = []jsonObject{}
		}
		r.json(jsonObject{"type", "summary", "success", len(failures) == 0, "errorCount", len(failures), "errors", failures})
	case r.minimal:
		r.overwrite("100% Done")
		fmt.Fprintln(r.out)
		r.line = 0
	}
}

// heartbeat prints {"type": "heartbeat"} every so often, until the function it gives is called.
func (r *reporter) heartbeat(every time.Duration) func() {
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				r.mutex.Lock()
				r.json(jsonObject{"type", "heartbeat", "time", float64(now.UnixMilli()) / 1000})
				r.mutex.Unlock()
			}
		}
	}()
	return func() {
		close(stop)
		<-stopped
	}
}

// errorCode is the code nsz gives an error of the action.
func (r *reporter) errorCode() int {
	switch r.action {
	case "decompress":
		return codeDecompress
	case "verify":
		return codeVerify
	}
	return codeCompress
}

// overwrite writes text over the line of progress of --minimal-output, with spaces where the
// line before was longer.
func (r *reporter) overwrite(text string) {
	length := utf8.RuneCountInString(text)
	fmt.Fprint(r.out, "\r"+text+strings.Repeat(" ", max(0, r.line-length)))
	r.line = length
}

// endLine clears the line of progress of --minimal-output, so a line for people starts clean.
func (r *reporter) endLine() {
	if r.line > 0 {
		fmt.Fprint(r.out, "\r"+strings.Repeat(" ", r.line)+"\r")
		r.line = 0
	}
}

// jsonObject is a JSON object as its keys and values in turn, to keep them in order.
type jsonObject []any

// json prints an object on a line, its keys in order and spaced as nsz prints them (Python's
// json.dumps): {"type": "progress", "step": ...}.
func (r *reporter) json(object jsonObject) {
	var line strings.Builder
	writeJSON(&line, object)
	line.WriteString("\n")
	io.WriteString(r.out, line.String())
}

func writeJSON(line *strings.Builder, value any) {
	switch value := value.(type) {
	case jsonObject:
		line.WriteString("{")
		for i := 0; i+1 < len(value); i += 2 {
			if i > 0 {
				line.WriteString(", ")
			}
			writeJSON(line, value[i])
			line.WriteString(": ")
			writeJSON(line, value[i+1])
		}
		line.WriteString("}")
	case []jsonObject:
		line.WriteString("[")
		for i, object := range value {
			if i > 0 {
				line.WriteString(", ")
			}
			writeJSON(line, object)
		}
		line.WriteString("]")
	default:
		var encoded bytes.Buffer
		encoder := json.NewEncoder(&encoded)
		encoder.SetEscapeHTML(false)
		encoder.Encode(value)
		line.WriteString(strings.TrimSuffix(encoded.String(), "\n"))
	}
}
