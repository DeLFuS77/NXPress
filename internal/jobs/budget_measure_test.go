package jobs

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// TestMeasureTotalOfThreads compares, on synthetic games, one big game and six small ones
// compressed with the same threads for each file (-t 3 -m 7) and with a total of 21 threads, the
// biggest first. It runs only when NXPRESS_MEASURE_TOTAL gives the size of the big game in MiB
// (the small ones an eighth of it each), at the level of NXPRESS_MEASURE_LEVEL (default 18), and
// logs how long each took and when each file ended. Each setting runs twice, in turns. With
// NXPRESS_MEASURE_NO_MEMORY set, the free memory lowers the threads of neither, so both work on
// 21 threads.
func TestMeasureTotalOfThreads(t *testing.T) {
	value := os.Getenv("NXPRESS_MEASURE_TOTAL")
	if value == "" {
		t.Skip("no measure asked (NXPRESS_MEASURE_TOTAL)")
	}
	big, err := strconv.Atoi(value)
	if err != nil || big < 8 {
		t.Fatalf("NXPRESS_MEASURE_TOTAL is the size of the big game in MiB, not %q", value)
	}
	level := DefaultLevel
	if value := os.Getenv("NXPRESS_MEASURE_LEVEL"); value != "" {
		level, _ = strconv.Atoi(value)
	}
	testkit.Isolate(t)
	folder := t.TempDir()
	games := []string{measureGame(t, folder, "big", big<<20, 1)}
	for i := range 6 {
		games = append(games, measureGame(t, folder, fmt.Sprintf("small%d", i), big<<20/8, 2+i))
	}
	// the memory taken to make the games goes back to the system, which the threads count on
	debug.FreeOSMemory()
	if os.Getenv("NXPRESS_MEASURE_NO_MEMORY") != "" {
		noMemoryLimit(t)
		before := limitThreads
		limitThreads = func(_ string, threads int, _ int) int { return threads }
		t.Cleanup(func() { limitThreads = before })
	}
	settings := []struct {
		name string
		o    Options
	}{
		{"-t 3 -m 7", Options{Threads: 3, Multi: 7, MultiGiven: true}},
		{"--total-threads 21 --biggest-first", Options{TotalThreads: 21, BiggestFirst: true}},
	}
	t.Logf("level %d, a game of %d MiB and six of %d MiB; %d threads for each file of -t 3 -m 7 by the memory now, %d in all with the total",
		level, big, big/8, limitThreads(strconv.Itoa(level), 3, 7), memoryThreads(level))
	for round := range 2 {
		for _, setting := range settings {
			for _, game := range games {
				os.Remove(strings.TrimSuffix(game, ".nsp") + ".nsz")
			}
			o := setting.o
			o.Compress, o.Level, o.Files = true, level, games
			if err := o.Check(); err != nil {
				t.Fatal(err)
			}
			out := &endTimes{start: time.Now(), ended: map[string]time.Duration{}}
			if failed := RunJobs(context.Background(), o, out); failed != 0 {
				t.Fatalf("%s: %d failed", setting.name, failed)
			}
			took := time.Since(out.start)
			ends := []string{}
			for _, game := range games {
				ends = append(ends, fmt.Sprintf("%s %.1f s", strings.TrimSuffix(filepath.Base(game), ".nsp"), out.ended[game].Seconds()))
			}
			t.Logf("round %d, %s: %.1f s (%s)%s", round+1, setting.name, took.Seconds(), strings.Join(ends, ", "), out.said)
		}
	}
}

// measureGame writes a game of about size bytes, one program NCA, whose data compresses about as
// a game does: a third of it random, the rest words.
func measureGame(t *testing.T, folder string, name string, size int, seed int) string {
	t.Helper()
	random := rand.New(rand.NewPCG(uint64(seed), 7))
	words := []string{}
	for range 4096 {
		word := make([]byte, 2+random.IntN(9))
		for i := range word {
			word[i] = byte('a' + random.IntN(26))
		}
		words = append(words, string(word))
	}
	data := make([]byte, size)
	for at := 0; at < size; {
		block := data[at:min(size, at+64<<10)]
		if random.IntN(3) == 0 {
			for i := range block {
				block[i] = byte(random.Uint32())
			}
		} else {
			for i := 0; i < len(block); {
				// the common words more often, as in a text
				word := words[min(random.IntN(4096), random.IntN(4096))]
				i += copy(block[i:], word+" ")
			}
		}
		at += len(block)
	}
	program := testkit.ProgramNcaWith(data, nil)
	data = nil
	return testkit.NspWith(t, filepath.Join(folder, name+".nsp"), testkit.BaseID+uint64(seed)<<12, 0, switchfs.ContentMetaType_Application, program, testkit.ContentName(program, ".nca"), nil)
}

// endTimes keeps when each file of a work ended, and what the work said.
type endTimes struct {
	testkit.Quiet
	mutex sync.Mutex
	start time.Time
	ended map[string]time.Duration
	said  []string
}

func (e *endTimes) Done(path string, output string, note string, err error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.ended[path] = time.Since(e.start)
}

func (e *endTimes) Say(format string, args ...any) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.said = append(e.said, fmt.Sprintf(format, args...))
}
