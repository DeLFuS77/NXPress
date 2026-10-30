package main

import (
	"strings"
	"testing"
)

// A notice clicked with NXPress closed starts it with -Embedding or -ToastActivated: the window
// opens, and the command line does not take them as unknown options.
func TestNotificationOpensTheWindow(t *testing.T) {
	for args, want := range map[string]bool{
		"-Embedding":            true,
		"-ToastActivated":       true,
		"-embedding":            true,
		"-ToastActivated extra": true,
		"-C a.nsp":              false,
		"--embedding":           false,
		"":                      false,
	} {
		if got := fromNotification(strings.Fields(args)); got != want {
			t.Errorf("%q: %v", args, got)
		}
	}
}
