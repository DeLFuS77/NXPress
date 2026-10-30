package main

import (
	"context"
	"testing"
	"time"
)

// Paused, the work waits before going on, until it goes on or is cancelled.
func TestWindowWaitsWhilePaused(t *testing.T) {
	app := &App{}
	job, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &window{job: job, app: app}
	waits := func() chan struct{} {
		released := make(chan struct{})
		go func() {
			w.waitWhilePaused()
			close(released)
		}()
		return released
	}

	app.paused.Store(true)
	released := waits()
	select {
	case <-released:
		t.Fatal("the work went on while paused")
	case <-time.After(400 * time.Millisecond):
	}
	app.paused.Store(false)
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("the work did not go on once it was no longer paused")
	}

	app.paused.Store(true)
	released = waits()
	cancel()
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled, the work still waited")
	}

	// not paused, it does not wait at all
	app.paused.Store(false)
	start := time.Now()
	(&window{job: context.Background(), app: app}).waitWhilePaused()
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("the work waited without a pause")
	}
}
