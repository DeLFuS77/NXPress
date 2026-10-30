//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// renameNew gives a file or folder a new name only when nothing has that name yet: it never
// replaces a file, as os.Rename does. Windows moves it without MOVEFILE_REPLACE_EXISTING.
func renameNew(from string, to string) error {
	source, err := windows.UTF16PtrFromString(longPath(from))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	target, err := windows.UTF16PtrFromString(longPath(to))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	if err := windows.MoveFileEx(source, target, 0); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}

// longPath lets Windows take a path longer than its 260 characters, as the os package does: an
// absolute path gets the \\?\ prefix, which also turns off the parsing that would drop the
// spaces and dots at its end.
func longPath(path string) string {
	if len(path) < 248 || !filepath.IsAbs(path) || strings.HasPrefix(path, `\\?\`) {
		return path
	}
	path = filepath.Clean(path)
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + path[2:]
	}
	return `\\?\` + path
}
