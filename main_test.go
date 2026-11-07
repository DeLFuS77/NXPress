package main

import (
	"os"
	"path/filepath"
	"slices"
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

// The window opens with no arguments, with files, with a notice, and with the files the
// Explorer gives with --open (#87) or --section and a section (#88), the files gone meanwhile
// left out; other arguments are for the command line.
func TestWindowLaunch(t *testing.T) {
	folder := t.TempDir()
	game := filepath.Join(folder, "game.nsp")
	if err := os.WriteFile(game, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(folder, "gone.nsz")
	for _, c := range []struct {
		args    []string
		window  bool
		files   []string
		section string
		open    bool
	}{
		{nil, true, nil, "", false},
		{[]string{game}, true, []string{game}, "", false},
		{[]string{folder}, true, []string{folder}, "", false},
		{[]string{"-Embedding"}, true, nil, "", false},
		{[]string{"--open", game}, true, []string{game}, "", true},
		{[]string{"--open", gone}, true, []string{}, "", true},
		{[]string{"--section", "verify", game, gone}, true, []string{game}, "verify", false},
		{[]string{"--section", "pack", folder}, true, []string{folder}, "pack", false},
		{[]string{"--section", "erase", game}, false, nil, "", false},
		{[]string{"--section", "verify"}, false, nil, "", false},
		{[]string{"--open"}, false, nil, "", false},
		{[]string{"-C", game}, false, nil, "", false},
		{[]string{gone}, false, nil, "", false},
	} {
		start, window := windowLaunch(c.args)
		if window != c.window {
			t.Errorf("%q: window %v", c.args, window)
			continue
		}
		if window && (!slices.Equal(start.files, c.files) || start.section != c.section || start.open != c.open) {
			t.Errorf("%q: %+v", c.args, start)
		}
	}
}

// The paths given to another NXPress are made whole from the folder it started in, and the
// options and the section are left as they are.
func TestAbsolute(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "games")
	whole := filepath.Join(folder, "a.nsp")
	for _, c := range []struct{ args, want []string }{
		{[]string{"a.nsp"}, []string{whole}},
		{[]string{"--section", "verify", "a.nsp"}, []string{"--section", "verify", whole}},
		{[]string{"--open", whole}, []string{"--open", whole}},
		{[]string{"-Embedding"}, []string{"-Embedding"}},
	} {
		if got := absolute(c.args, folder); !slices.Equal(got, c.want) {
			t.Errorf("%q: %q", c.args, got)
		}
	}
}
