//go:build !windows

package main

import (
	"errors"
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

// sendToFolder is empty: only the Explorer of Windows has a "Send to" menu.
func sendToFolder() string { return "" }

func sendToEnabled() bool { return false }

func enableSendTo() error { return errors.New("only Windows has a Send to menu") }

func disableSendTo() error { return nil }

// archiveBit cannot be set here: FAT32 attributes are set on the card itself.
func archiveBit(path string) error { return errors.New("no archive bit here") }
