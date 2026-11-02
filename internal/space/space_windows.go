//go:build windows

package space

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// VolumeOf names the drive of a folder: its letter or its share.
func VolumeOf(folder string) string {
	if volume := filepath.VolumeName(folder); volume != "" {
		return strings.ToLower(volume)
	}
	return folder
}

// FreeSpace is the room left on the drive of a folder for this user.
func FreeSpace(folder string) (int64, error) {
	path, err := windows.UTF16PtrFromString(existing(folder))
	if err != nil {
		return 0, err
	}
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(path, &available, nil, nil); err != nil {
		return 0, err
	}
	return int64(available), nil
}
