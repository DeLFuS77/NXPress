//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows"
)

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
