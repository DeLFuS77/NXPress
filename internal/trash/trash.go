// Package trash sends the files and folders NXPress is done with to the recycle bin of the system
// instead of deleting them for good: the Recycle Bin of Windows, the Trash of macOS and the trash
// of the freedesktop.org specification on Linux. A drive without one (a network folder, some USB
// drives) is told apart from a move that failed, so the caller chooses what to do with what
// stays.
package trash

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// The reasons a path cannot go to the recycle bin.
const (
	NoBin  = "its drive has no recycle bin"
	TooBig = "it is too big for the recycle bin of its drive"
	// Windows: a path longer than its Recycle Bin takes
	TooLong = "its path is too long for the recycle bin"
)

// Unavailable is why a path cannot go to the recycle bin, and the drive it is on (C:\, a network
// share, the folder a drive is mounted on); the path stays where it was.
type Unavailable struct {
	Reason string
	Drive  string
}

func (e *Unavailable) Error() string {
	return e.Reason
}

// Why gives the reason of an error that tells a path cannot go to the recycle bin.
func Why(err error) (*Unavailable, bool) {
	var unavailable *Unavailable
	if errors.As(err, &unavailable) {
		return unavailable, true
	}
	return nil, false
}

// Move sends path, a file or a folder, to the recycle bin of its drive. An *Unavailable error
// tells there is none for it; any other error that it could not be moved. Either way the path is
// where it was.
func Move(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(absolute); err != nil {
		return err
	}
	return move(absolute)
}

// Check tells, without moving it, whether path can go to the recycle bin: nil when it can, an
// *Unavailable when its drive has none or it does not fit in it, another error when the path
// cannot be read.
func Check(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(absolute); err != nil {
		return err
	}
	return check(absolute)
}

// sizeOf is the size of a file, or of every file in a folder.
func sizeOf(path string) int64 {
	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}
	var total int64
	filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			if info, err := entry.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
