package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// commitFile gives a file written under a temporary name its own name for good: its bytes
// reach the disk before the rename, and the rename before the original is deleted, so a power
// cut never leaves the new file empty or missing with the original already gone.
func commitFile(temporary string, target string) error {
	if err := syncPath(temporary); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	syncFolder(filepath.Dir(target))
	return nil
}

// syncPath waits until a file written and closed is on the disk; for a folder (the parts of a
// FAT32 split), every file in it and the folder itself.
func syncPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := syncPath(filepath.Join(path, entry.Name())); err != nil {
				return err
			}
		}
		syncFolder(path)
		return nil
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// syncFolder writes the names of a folder to the disk, a rename among them. Windows has no way
// to, and keeps them in the journal of NTFS on its own.
func syncFolder(folder string) {
	if runtime.GOOS == "windows" {
		return
	}
	if file, err := os.Open(folder); err == nil {
		file.Sync()
		file.Close()
	}
}
