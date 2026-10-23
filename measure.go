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

// measureTrials are the settings tried, the lightest first: half the processors and all of
// them, on one, two and four files at once, each only when its threads fit in the memory.
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

// chooseTrial gives the fastest setting tried; a heavier one must be clearly faster than a
// lighter one, as the measure varies a little.
func chooseTrial(trials []trial) trial {
	best := trials[0]
	for _, t := range trials[1:] {
		if float64(t.speed) > float64(best.speed)*1.1 {
			best = t
		}
	}
	return best
}

// measured is told to the window once a measure chose its setting, to apply it.
type measured interface {
	measured(threads int, files int)
}

// measure tries settings of the compression on part of a game, as a compression reads,
// decrypts, compresses and writes it, at the level and the kind chosen, and tells the
// fastest: the threads and the files at once to give -t and -m. Each file of each trial
// reads its own part of the game, so none finds the data already in memory.
func measure(ctx context.Context, path string, o options, out output) (string, string, error) {
	level := strconv.Itoa(o.level)
	trials := measureTrials(runtime.NumCPU(), level)
	places := 0
	for _, t := range trials {
		places += t.files
	}
	// the files are written where the compressed files would go, and deleted
	folder := filepath.Dir(path)
	if o.output != "" {
		folder = o.output
	}
	progress := out.progress(path, "Measuring")
	place := 0
	for i := range trials {
		var (
			total int64
			mutex sync.Mutex
			wait  sync.WaitGroup
			first error
		)
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
			return "", "nothing in it can be compressed, it cannot be measured with", nil
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
	if window, ok := out.(measured); ok {
		window.measured(best.threads, best.files)
	}
	return "", fmt.Sprintf("best: -t %d -m %d, %d MB/s, about %s for 10 GB", best.threads, best.files, best.speed,
		duration(time.Duration(float64(10<<10)/float64(max(1, best.speed))*float64(time.Second)))), nil
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
