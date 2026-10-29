package main

import (
	"context"
	"testing"
)

// While the work is paused the computer may go to sleep; it is kept awake again as the work goes
// on, and let go once the work ends.
func TestPauseLetsTheComputerSleep(t *testing.T) {
	held := 0
	blocked := blockSleep
	blockSleep = func() func() {
		held++
		return func() { held-- }
	}
	defer func() { blockSleep = blocked }()

	release := keepAwake()
	if held != 1 {
		t.Fatalf("at work: %d holds, want 1", held)
	}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &App{cancel: cancel}
	a.SetPaused(true)
	if held != 0 {
		t.Fatal("paused, the computer is still kept awake")
	}
	a.SetPaused(false)
	if held != 1 {
		t.Fatal("going on, the computer is not kept awake again")
	}
	// cancelled while paused, the pause ends
	a.SetPaused(true)
	a.Cancel()
	if held != 1 {
		t.Fatal("cancelled while paused, the computer is not kept awake until the work ends")
	}
	// two works at once: the computer is kept awake until both end
	second := keepAwake()
	release()
	release()
	if held != 1 {
		t.Fatalf("one of two works ended: %d holds, want 1", held)
	}
	second()
	if held != 0 {
		t.Fatalf("the work ended: %d holds, want 0", held)
	}
}
