//go:build !windows

package main

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// stampOf reads what tells a file apart: its device, its inode and when the inode changed last.
func stampOf(path string) (fileStamp, error) {
	var stat unix.Stat_t
	err := unix.Stat(path, &stat)
	for err == unix.EINTR {
		err = unix.Stat(path, &stat)
	}
	if err != nil {
		return fileStamp{}, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	return fileStamp{
		Size:     int64(stat.Size),
		Modified: time.Unix(stat.Mtim.Unix()),
		Volume:   uint64(stat.Dev),
		Index:    uint64(stat.Ino),
		Changed:  time.Unix(stat.Ctim.Unix()),
		Folder:   uint32(stat.Mode)&unix.S_IFMT == unix.S_IFDIR,
	}, nil
}
