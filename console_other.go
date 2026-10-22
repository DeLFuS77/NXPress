//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// hideConsole has nothing to do: only Windows opens a console for a program started from the
// file manager.
func hideConsole() {}

// showFile opens the folder of a file in the file manager.
func showFile(path string) {
	if runtime.GOOS == "darwin" {
		exec.Command("open", "-R", path).Start()
		return
	}
	exec.Command("xdg-open", filepath.Dir(path)).Start()
}
