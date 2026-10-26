package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// measureTrialTime is how long each setting is tried: six settings take about half a minute.
const measureTrialTime = 4 * time.Second

// trial is a setting tried by --measure: threads per file and files at once, and the MB of
// the game they compressed per second, all files together.
type trial struct {
	threads int
	files   int
	speed   int
}

// measureTrials are the settings tried, the lightest first: half the processors and all the
// usable ones (see usableProcessors), on one, two and four files at once, each only when its
// threads fit in the memory.
func measureTrials(processors int, level string) []trial {
	trials := []trial{}
	seen := map[[2]int]bool{}
	for _, total := range []int{max(1, processors/2), processors} {
		for _, files := range []int{1, 2, 4} {
			threads := total / files
			if threads < 1 || seen[[2]int{threads, files}] || switchfs.LimitThreads(level, threads, files) < threads {
				continue
			}
			seen[[2]int{threads, files}] = true
			trials = append(trials, trial{threads: threads, files: files})
		}
	}
	if len(trials) == 0 {
		trials = append(trials, trial{threads: 1, files: 1})
	}
	return trials
}

// measureMargin is how much faster a heavier setting must be than a lighter one to be chosen,
// as the measure varies a little from one run to the next: 5%.
const measureMargin = 1.05

// chooseTrial gives the fastest setting tried; a heavier one must be at least 5% faster than a
// lighter one.
func chooseTrial(trials []trial) trial {
	best := trials[0]
	for _, t := range trials[1:] {
		if float64(t.speed) >= float64(best.speed)*measureMargin {
			best = t
		}
	}
	return best
}

// fasterTrial is the fastest setting tried when it was not chosen, less than 5% faster than the
// one chosen, so the window can tell why the lighter one stays; or nil.
func fasterTrial(trials []trial, best trial) *trial {
	var faster *trial
	for i := range trials {
		if trials[i].speed > best.speed && (faster == nil || trials[i].speed > faster.speed) {
			faster = &trials[i]
		}
	}
	return faster
}

// measureReport is what a measure found: every setting tried and the one chosen, which the
// window applies and shows.
type measureReport struct {
	processors int
	level      int
	block      bool
	trials     []trial
	best       trial
	// the fastest setting when it was not chosen, or nil
	faster *trial
	// how long 10 GB of games take with the setting chosen
	per10GB time.Duration
}

// measured is told to the window once a measure chose its setting, to apply and show it.
type measured interface {
	measured(report measureReport)
}

// measure tries settings of the compression on part of a game, as a compression reads,
// decrypts, compresses and writes it, at the level and the kind chosen, and tells the
// fastest: the threads and the files at once to give -t and -m. Each file of each trial
// reads its own part of the game, so none finds the data already in memory.
func measure(ctx context.Context, path string, o options, out output) (string, string, error) {
	level := strconv.Itoa(o.level)
	trials := measureTrials(usableProcessors(), level)
	places := 0
	for _, t := range trials {
		places += t.files
	}
	// the files are written where the compressed files would go, and deleted
	folder := filepath.Dir(path)
	if o.output != "" {
		folder = o.output
	}
	place := 0
	for i := range trials {
		var (
			total int64
			mutex sync.Mutex
			wait  sync.WaitGroup
			first error
		)
		// each trial tells it starts, "Measuring 2/6", as one takes from a few seconds to half
		// a minute: the file does not look left waiting meanwhile
		progress := out.progress(path, fmt.Sprintf("Measuring %d/%d", i+1, len(trials)))
		progress(int64(i), int64(len(trials)))
		start := time.Now()
		for f := 0; f < trials[i].files; f++ {
			wait.Add(1)
			go func(from float64) {
				defer wait.Done()
				done, err := measureOne(ctx, path, folder, from, switchfs.CompressOptions{Level: level, Workers: trials[i].threads, Blocks: o.block})
				mutex.Lock()
				defer mutex.Unlock()
				total += done
				if err != nil && first == nil {
					first = err
				}
			}(float64(place) / float64(places))
			place++
		}
		wait.Wait()
		if errors.Is(first, switchfs.ErrNothingToMeasure) {
			return "", "", skipped("nothing in it can be compressed, it cannot be measured with")
		}
		if first != nil {
			return "", "", first
		}
		trials[i].speed = int(float64(total) / (1 << 20) / time.Since(start).Seconds())
		progress(int64(i+1), int64(len(trials)))
	}
	best := chooseTrial(trials)
	lines := []string{}
	for _, t := range trials {
		lines = append(lines, fmt.Sprintf("%d × %d: %d MB/s", t.threads, t.files, t.speed))
	}
	out.say("%d processors, level %d (threads × files at once):\n  %s", runtime.NumCPU(), o.level, strings.Join(lines, "\n  "))
	per10GB := time.Duration(float64(10<<10) / float64(max(1, best.speed)) * float64(time.Second))
	if window, ok := out.(measured); ok {
		window.measured(measureReport{processors: runtime.NumCPU(), level: o.level, block: o.block, trials: trials, best: best, faster: fasterTrial(trials, best), per10GB: per10GB})
	}
	return "", fmt.Sprintf("best: -t %d -m %d, %d MB/s, about %s for 10 GB", best.threads, best.files, best.speed, duration(per10GB)), nil
}

// measurableGame is the first of the games given that a measure can compress, read without
// compressing anything. When none can, the first one is skipped with why; when one could not
// be read at all (damaged, no keys for it), that error is told instead.
func measurableGame(ctx context.Context, files []string, o options) (string, error) {
	options := switchfs.CompressOptions{Level: strconv.Itoa(o.level), Workers: 1, Blocks: o.block}
	var failed error
	failedPath := ""
	for _, path := range files {
		_, err := switchfs.MeasureCompression(ctx, path, 0, options, 0, io.Discard)
		if err == nil {
			return path, nil
		}
		if ctx.Err() != nil {
			return path, ctx.Err()
		}
		if !errors.Is(err, switchfs.ErrNothingToMeasure) && failed == nil {
			failed, failedPath = err, path
		}
	}
	if failed != nil {
		return failedPath, failed
	}
	reason := "nothing in it can be compressed, it cannot be measured with"
	if len(files) > 1 {
		reason = "nothing in the games of the list can be compressed, none can be measured with"
	}
	return files[0], skipped(reason)
}

// measureOne compresses a part of the game for a trial into a hidden file in folder, and
// deletes it; where nothing can be written, the measure leaves the writing out.
func measureOne(ctx context.Context, path string, folder string, from float64, options switchfs.CompressOptions) (int64, error) {
	var output io.Writer = io.Discard
	if file, err := os.CreateTemp(folder, ".nxpress-measure-*"); err == nil {
		defer os.Remove(file.Name())
		defer file.Close()
		output = file
	}
	return switchfs.MeasureCompression(ctx, path, from, options, measureTrialTime, output)
}
