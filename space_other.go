//go:build !windows

package main

import "golang.org/x/sys/unix"

// freeSpace is the room left on the drive of a folder for this user.
func freeSpace(folder string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(existing(folder), &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
