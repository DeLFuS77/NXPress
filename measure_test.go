package main

import (
	"runtime"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// A heavier setting is chosen when it is at least 5% faster; one a little faster is told, so the
// window says why the lighter one stays.
func TestMeasureMargin(t *testing.T) {
	light, heavy := switchfs.MeasureTrial{Threads: 14, Files: 1, Speed: 55}, switchfs.MeasureTrial{Threads: 7, Files: 2, Speed: 60}
	if chosen := switchfs.ChooseTrial([]switchfs.MeasureTrial{light, heavy}); chosen != heavy {
		t.Fatalf("9%% faster is chosen: %v", chosen)
	}
	near := switchfs.MeasureTrial{Threads: 7, Files: 2, Speed: 57}
	trials := []switchfs.MeasureTrial{light, near, {Threads: 3, Files: 4, Speed: 55}}
	chosen := switchfs.ChooseTrial(trials)
	if chosen != light {
		t.Fatalf("less than 5%% faster is not chosen: %v", chosen)
	}
	if faster := fasterTrial(trials, chosen); faster == nil || *faster != near {
		t.Fatalf("the faster one not taken is told: %v", faster)
	}
	if faster := fasterTrial([]switchfs.MeasureTrial{light, heavy}, heavy); faster != nil {
		t.Fatalf("nothing to tell when the fastest is chosen: %v", faster)
	}
}

// A margin of the processors is left to the rest of the computer, at least one.
func TestUsableProcessorsLeaveAMargin(t *testing.T) {
	if n := switchfs.UsableProcessors(); runtime.NumCPU() > 1 && (n >= runtime.NumCPU() || n < runtime.NumCPU()*7/8-1) {
		t.Fatalf("%d of %d processors", n, runtime.NumCPU())
	}
	if threads := defaultThreads(2); threads != max(1, switchfs.UsableProcessors()/2) {
		t.Fatalf("%d threads for each of two files", threads)
	}
}
