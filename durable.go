package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// commitFile gives a file written under a temporary name its own name for good: its bytes
// reach the disk before the rename, and the rename before the original is deleted, so a power
// cut never leaves the new file empty or missing with the original already gone. A file with
// that name is replaced.
func commitFile(temporary string, target string) error {
	return commit(temporary, target, os.Rename)
}

// commitNew is commitFile that never replaces a file: it fails when one has the name already.
func commitNew(temporary string, target string) error {
	return commit(temporary, target, renameNew)
}

func commit(temporary string, target string, rename func(string, string) error) error {
	if err := syncPath(temporary); err != nil {
		return err
	}
	if err := rename(temporary, target); err != nil {
		return err
	}
	syncFolder(filepath.Dir(target))
	return nil
}

// errOriginalAside tells that a place failed and could not put the original back: it is kept
// under its hidden name, and the new copy is kept too.
var errOriginalAside = errors.New("the original is kept as")

// place gives a file or folder written under a temporary name its own name for good, as
// commitFile does, where a file or folder may be already: the original worked on in place, or
// what -w replaces (a folder of parts, which a rename does not replace). The one there is moved
// aside under a hidden name first, and deleted only once the new one has its name; when that
// fails, it is put back.
func place(temporary string, target string) error {
	if err := syncPath(temporary); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err != nil {
		return commitNew(temporary, target)
	}
	aside := filepath.Join(filepath.Dir(target), fmt.Sprintf(".%s.%d.nxpress-old", filepath.Base(target), time.Now().UnixNano()))
	if err := renameNew(target, aside); err != nil {
		return err
	}
	if err := renameNew(temporary, target); err != nil {
		if back := renameNew(aside, target); back != nil {
			return fmt.Errorf("%w; %w %s", err, errOriginalAside, aside)
		}
		return err
	}
	syncFolder(filepath.Dir(target))
	if err := os.RemoveAll(aside); err != nil {
		return fmt.Errorf("the new file is checked, but the original could not be deleted: %w", err)
	}
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
