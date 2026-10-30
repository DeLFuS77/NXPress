//go:build !windows

package main

import (
	"errors"
	"io/fs"
	"os"
)

// renameNew gives a file or folder a new name only when nothing has that name yet: it never
// replaces a file, as os.Rename does. A hard link fails when the name is taken, and the old
// name goes once the new one is there; on a file system without hard links (FAT, exFAT, some
// network shares), and for a folder, the name is looked at first.
func renameNew(from string, to string) error {
	err := os.Link(from, to)
	if err == nil {
		os.Remove(from)
		return nil
	}
	if errors.Is(err, fs.ErrExist) {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: fs.ErrExist}
	}
	if _, err := os.Lstat(to); err == nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: fs.ErrExist}
	}
	return os.Rename(from, to)
}
