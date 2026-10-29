package main

import (
	"context"
	"testing"
	"time"
)

// These tests never let a turning off come: they look at what decides it, not at shutDown.

// New work stops the turning off asked for once the work before was done, and tells the page.
func TestStartStopsTheShutdown(t *testing.T) {
	events := catchEvents(t)
	a := &App{ctx: context.Background()}
	a.shutdown = time.AfterFunc(time.Hour, func() {})
	a.shutdownRound = 1
	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true}); err != nil {
		t.Fatal(err)
	}
	defer a.closed(context.Background())
	a.mutex.Lock()
	pending := a.shutdown != nil
	a.mutex.Unlock()
	if pending {
		t.Fatal("the turning off goes on with new work")
	}
	if a.shutdownDue(1) {
		t.Fatal("the turning off stopped is still due")
	}
	eventually(t, "the page is not told the turning off is cancelled", func() bool { return events.count("shutdownCancelled") == 1 })
}

// As its time comes, the turning off is not done with files at work, nor once cancelled or asked
// again.
func TestShutdownWaitsForNoWork(t *testing.T) {
	events := catchEvents(t)
	a := &App{ctx: context.Background()}
	ask := func() int {
		a.mutex.Lock()
		defer a.mutex.Unlock()
		a.stopShutdown()
		a.shutdownRound++
		a.shutdown = time.AfterFunc(time.Hour, func() {})
		return a.shutdownRound
	}

	round := ask()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.cancel = cancel
	if a.shutdownDue(round) {
		t.Fatal("the computer turns off with files at work")
	}
	if events.count("shutdownCancelled") != 1 {
		t.Fatal("the page is not told the turning off is cancelled")
	}
	if a.shutdown != nil {
		t.Fatal("the turning off not done is still pending")
	}

	a.cancel = nil
	round = ask()
	if !a.shutdownDue(round) {
		t.Fatal("the turning off is not done without work")
	}
	if a.shutdownDue(round) {
		t.Fatal("the turning off is done twice")
	}

	old := ask()
	round = ask()
	if a.shutdownDue(old) {
		t.Fatal("a turning off asked again is done at its first time")
	}
	a.CancelShutDown()
	if a.shutdownDue(round) {
		t.Fatal("a turning off cancelled is done")
	}
}
