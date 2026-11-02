//go:build !windows

package system

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
)

// HideConsole has nothing to do: only Windows opens a console for a program started from the
// file manager.
func HideConsole() {}

// ShowFile opens the folder of a file in the file manager. The command is waited for, so it
// leaves no process behind once it ends.
func ShowFile(path string) {
	command := exec.Command("xdg-open", filepath.Dir(path))
	if runtime.GOOS == "darwin" {
		command = exec.Command("open", "-R", path)
	}
	if command.Start() == nil {
		go command.Wait()
	}
}

// SendToFolder is empty: only the Explorer of Windows has a "Send to" menu.
func SendToFolder() string { return "" }

func SendToEnabled() bool { return false }

func EnableSendTo() error { return errors.New("only Windows has a Send to menu") }

func DisableSendTo() error { return nil }

// ArchiveBit cannot be set here: FAT32 attributes are set on the card itself.
func ArchiveBit(path string) error { return errors.New("no archive bit here") }
