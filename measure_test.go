package main

import (
	"runtime"
	"testing"
)

// The settings tried use half the processors and all the usable ones, on one, two and four files,
// each once, the lightest first; the fastest wins only when it is clearly faster.
func TestMeasureTrials(t *testing.T) {
	trials := measureTrials(8, "3")
	want := []trial{{threads: 4, files: 1}, {threads: 2, files: 2}, {threads: 1, files: 4}, {threads: 8, files: 1}, {threads: 4, files: 2}, {threads: 2, files: 4}}
	if len(trials) != len(want) {
		t.Fatalf("trials %v", trials)
	}
	for i := range want {
		if trials[i] != want[i] {
			t.Fatalf("trial %d: %v, want %v", i, trials[i], want[i])
		}
	}
	if got := measureTrials(1, "3"); len(got) != 1 || got[0] != (trial{threads: 1, files: 1}) {
		t.Fatalf("one processor: %v", got)
	}
	chosen := chooseTrial([]trial{{threads: 4, files: 1, speed: 100}, {threads: 8, files: 1, speed: 105}, {threads: 4, files: 2, speed: 120}})
	if chosen != (trial{threads: 4, files: 2, speed: 120}) {
		t.Fatalf("chosen %v", chosen)
	}
}

// A heavier setting is chosen when it is at least 5% faster; one a little faster is told, so the
// window says why the lighter one stays.
func TestMeasureMargin(t *testing.T) {
	light, heavy := trial{threads: 14, files: 1, speed: 55}, trial{threads: 7, files: 2, speed: 60}
	if chosen := chooseTrial([]trial{light, heavy}); chosen != heavy {
		t.Fatalf("9%% faster is chosen: %v", chosen)
	}
	near := trial{threads: 7, files: 2, speed: 57}
	trials := []trial{light, near, {threads: 3, files: 4, speed: 55}}
	chosen := chooseTrial(trials)
	if chosen != light {
		t.Fatalf("less than 5%% faster is not chosen: %v", chosen)
	}
	if faster := fasterTrial(trials, chosen); faster == nil || *faster != near {
		t.Fatalf("the faster one not taken is told: %v", faster)
	}
	if faster := fasterTrial([]trial{light, heavy}, heavy); faster != nil {
		t.Fatalf("nothing to tell when the fastest is chosen: %v", faster)
	}
}

// A margin of the processors is left to the rest of the computer, at least one.
func TestUsableProcessorsLeaveAMargin(t *testing.T) {
	if n := usableProcessors(); runtime.NumCPU() > 1 && (n >= runtime.NumCPU() || n < runtime.NumCPU()*7/8-1) {
		t.Fatalf("%d of %d processors", n, runtime.NumCPU())
	}
	if threads := defaultThreads(2); threads != max(1, usableProcessors()/2) {
		t.Fatalf("%d threads for each of two files", threads)
	}
}
