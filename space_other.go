//go:build !windows

package main

import (
	"strconv"

	"golang.org/x/sys/unix"
)

// volumeOf names the drive of a folder by the device of the nearest folder that exists: every
// drive is a folder of the one tree, and many folders are on one drive.
func volumeOf(folder string) string {
	var stat unix.Stat_t
	if err := unix.Stat(existing(folder), &stat); err != nil {
		return folder
	}
	return "device " + strconv.FormatUint(uint64(stat.Dev), 10)
}

// freeSpace is the room left on the drive of a folder for this user.
func freeSpace(folder string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(existing(folder), &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
