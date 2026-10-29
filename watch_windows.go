//go:build windows

package main

import (
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// fileBusy tells whether another program has a file open to write, as a copy has it until it
// ends: then the file cannot be opened without letting others write to it.
func fileBusy(path string) bool {
	// a long path is given as such, as Go does for its own files
	if absolute, err := filepath.Abs(path); err == nil && len(absolute) >= 248 {
		switch {
		case strings.HasPrefix(absolute, `\\?\`):
			path = absolute
		case strings.HasPrefix(absolute, `\\`):
			path = `\\?\UNC\` + absolute[2:]
		default:
			path = `\\?\` + absolute
		}
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return errors.Is(err, windows.ERROR_SHARING_VIOLATION)
	}
	windows.CloseHandle(handle)
	return false
}
