//go:build windows

package main

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// a path longer than 260 characters opens by its \\?\ form, on a share by \\?\UNC\
func TestCheckCacheLongPathForm(t *testing.T) {
	long := strings.Repeat(`folder\`, 40) + "a.nsp"
	for path, want := range map[string]string{
		`C:\Games\a.nsp`:      `C:\Games\a.nsp`,
		`C:\` + long:          `\\?\C:\` + long,
		`C:\Games\..\` + long: `\\?\C:\` + long,
		`\\nas\games\` + long: `\\?\UNC\nas\games\` + long,
		`\\?\C:\` + long:      `\\?\C:\` + long,
		`Games\` + long:       `Games\` + long,
	} {
		if got := longPath(path); got != want {
			t.Errorf("%s: %s, want %s", path, got, want)
		}
	}
}

// notPlugged is the root of a drive that is not here: a letter no drive has
func notPlugged(t *testing.T) string {
	drives, err := windows.GetLogicalDrives()
	if err != nil {
		t.Fatal(err)
	}
	for letter := 'Z'; letter >= 'D'; letter-- {
		if drives&(1<<(letter-'A')) == 0 {
			return string(letter) + `:\`
		}
	}
	t.Skip("every drive letter is taken")
	return ""
}
