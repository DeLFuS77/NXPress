package main

import (
	"os"
	"slices"
	"syscall"
	"testing"
)

// The work stops on Ctrl+C and on SIGTERM, which Go also gives on Windows when the console is
// closed: the half written files are removed either way.
func TestCommandLineStopsOnSigterm(t *testing.T) {
	if !slices.Contains(stopSignals, os.Signal(syscall.SIGTERM)) || !slices.Contains(stopSignals, os.Interrupt) {
		t.Fatalf("signals: %v", stopSignals)
	}
}
