//go:build windows

package system

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// RenameNew gives a file or folder a new name only when nothing has that name yet: it never
// replaces a file, as os.Rename does. Windows moves it without MOVEFILE_REPLACE_EXISTING.
func RenameNew(from string, to string) error {
	source, err := windows.UTF16PtrFromString(LongPath(from))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	target, err := windows.UTF16PtrFromString(LongPath(to))
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	if err := windows.MoveFileEx(source, target, 0); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}

// LongPath lets Windows take a path longer than its 260 characters, as the os package does: an
// absolute path gets the \\?\ prefix, which also turns off the parsing that would drop the
// spaces and dots at its end.
func LongPath(path string) string {
	if len(path) < 248 || !filepath.IsAbs(path) || strings.HasPrefix(path, `\\?\`) {
		return path
	}
	path = filepath.Clean(path)
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + path[2:]
	}
	return `\\?\` + path
}
