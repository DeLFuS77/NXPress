package main

import "testing"

// The settings tried use half the processors and all of them, on one, two and four files,
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
