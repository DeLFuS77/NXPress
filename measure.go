package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// measureTrialTime is how long each setting is tried: six settings take about half a minute.
const measureTrialTime = 4 * time.Second

// measureWarmUp is how long the first setting runs before the trials, its speed left out: the
// first compression of NXPress is slower (its memory taken, the disk and the processor waking
// up), and a first trial came out at 1 MB/s where the others gave 84 to 479.
const measureWarmUp = 2 * time.Second

// fasterTrial is the fastest setting tried when it was not chosen, less than 5% faster than the
// one chosen, so the window can tell why the lighter one stays; or nil.
func fasterTrial(trials []switchfs.MeasureTrial, best switchfs.MeasureTrial) *switchfs.MeasureTrial {
	var faster *switchfs.MeasureTrial
	for i := range trials {
		if trials[i].Speed > best.Speed && (faster == nil || trials[i].Speed > faster.Speed) {
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
	trials     []switchfs.MeasureTrial
	best       switchfs.MeasureTrial
	// the fastest setting when it was not chosen, or nil
	faster *switchfs.MeasureTrial
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
	// on one, two and four files at once
	trials := switchfs.MeasureTrials(switchfs.UsableProcessors(), level, 4)
	places := 0
	for _, t := range trials {
		places += t.Files
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
		if i == 0 {
			// from the end of the game, which the trials read last, so the first one does not
			// find its part in memory
			_, err := measureOne(ctx, path, folder, (float64(places)-0.5)/float64(places), switchfs.CompressOptions{Level: level, Workers: trials[0].Threads, Blocks: o.block}, measureWarmUp)
			if errors.Is(err, switchfs.ErrNothingToMeasure) {
				return "", "", notMeasurable(path)
			}
			if err != nil {
				return "", "", err
			}
		}
		start := time.Now()
		for f := 0; f < trials[i].Files; f++ {
			wait.Add(1)
			go func(from float64) {
				defer wait.Done()
				done, err := measureOne(ctx, path, folder, from, switchfs.CompressOptions{Level: level, Workers: trials[i].Threads, Blocks: o.block}, measureTrialTime)
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
			return "", "", notMeasurable(path)
		}
		if first != nil {
			return "", "", first
		}
		trials[i].Speed = int(float64(total) / (1 << 20) / time.Since(start).Seconds())
		progress(int64(i+1), int64(len(trials)))
	}
	best := switchfs.ChooseTrial(trials)
	lines := []string{}
	for _, t := range trials {
		lines = append(lines, fmt.Sprintf("%d × %d: %d MB/s", t.Threads, t.Files, t.Speed))
	}
	out.say("%d processors, level %d (threads × files at once):\n  %s", runtime.NumCPU(), o.level, strings.Join(lines, "\n  "))
	per10GB := time.Duration(float64(10<<10) / float64(max(1, best.Speed)) * float64(time.Second))
	if window, ok := out.(measured); ok {
		window.measured(measureReport{processors: runtime.NumCPU(), level: o.level, block: o.block, trials: trials, best: best, faster: fasterTrial(trials, best), per10GB: per10GB})
	}
	return "", fmt.Sprintf("best: -t %d -m %d, %d MB/s, about %s for 10 GB", best.Threads, best.Files, best.Speed, duration(per10GB)), nil
}

// measurableGame is the biggest of the games given that a measure can compress, read without
// compressing anything: a small game is read whole in a trial, which then measures the start
// of a compression more than the compression. When none can, the biggest one is skipped with
// why; when one could not be read at all (damaged, no keys for it), that error is told instead.
func measurableGame(ctx context.Context, files []string, o options) (string, error) {
	options := switchfs.CompressOptions{Level: strconv.Itoa(o.level), Workers: 1, Blocks: o.block}
	sizes := map[string]int64{}
	for _, path := range files {
		if info, err := os.Stat(path); err == nil {
			sizes[path] = info.Size()
		}
	}
	files = slices.Clone(files)
	sort.SliceStable(files, func(i, j int) bool { return sizes[files[i]] > sizes[files[j]] })
	var failed error
	failedPath := ""
	for _, path := range files {
		// an empty file is told as such, as the other actions tell it, not as one that cannot be read
		err := errEmpty
		if !isEmpty(path) {
			_, err = switchfs.MeasureCompression(ctx, path, 0, options, 0, io.Discard)
		}
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
	if len(files) > 1 {
		return files[0], skipped("nothing in the games of the list can be compressed, none can be measured with")
	}
	return files[0], notMeasurable(files[0])
}

// notMeasurable skips a game the measure cannot compress, with why as a compression tells it:
// sparse storage, compressed already...
func notMeasurable(path string) error {
	reason := "nothing in it can be compressed, it cannot be measured with"
	if err := switchfs.NotCompressible(path); errors.Is(err, switchfs.ErrNotCompressible) {
		if detail := strings.TrimPrefix(err.Error(), switchfs.ErrNotCompressible.Error()+": "); detail != err.Error() {
			reason += ": " + detail
		}
	}
	return skipped(reason)
}

// measureOne compresses a part of the game for a trial, for about the time given, into a hidden
// file in folder, and deletes it; where nothing can be written, the measure leaves the writing
// out.
func measureOne(ctx context.Context, path string, folder string, from float64, options switchfs.CompressOptions, trial time.Duration) (int64, error) {
	var output io.Writer = io.Discard
	if file, err := os.CreateTemp(folder, ".nxpress-measure-*"); err == nil {
		defer os.Remove(file.Name())
		defer file.Close()
		output = file
	}
	return switchfs.MeasureCompression(ctx, path, from, options, trial, output)
}
