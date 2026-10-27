//go:build windows

package main

import "golang.org/x/sys/windows"

// freeSpace is the room left on the drive of a folder for this user.
func freeSpace(folder string) (int64, error) {
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
