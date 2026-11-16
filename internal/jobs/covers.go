package jobs

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/DeLFuS77/NXPress/internal/nup"
)

// covers are the covers the person chose in the window of Combine, by dump folder: each a JPEG of
// 256 × 256 pixels (nup.CheckCover) that the NUP of the folder takes as its icon. They are kept
// from the moment they are chosen, and the work that packs the folder reads its own.
var covers = struct {
	sync.RWMutex
	byFolder map[string][]byte
}{byFolder: map[string][]byte{}}

// coverKey is the place of a dump folder as a place, not as it was typed.
func coverKey(dir string) string {
	if absolute, err := filepath.Abs(dir); err == nil {
		dir = absolute
	}
	dir = filepath.Clean(dir)
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
	}
	return dir
}

// SetCover gives the NUP of a dump folder the cover given; an empty cover takes it away. A cover
// that is not one is refused.
func SetCover(dir string, cover []byte) error {
	if len(cover) > 0 {
		if err := nup.CheckCover(cover); err != nil {
			return err
		}
	}
	covers.Lock()
	defer covers.Unlock()
	if len(cover) == 0 {
		delete(covers.byFolder, coverKey(dir))
	} else {
		covers.byFolder[coverKey(dir)] = cover
	}
	return nil
}

// coverFor is the cover chosen for a dump folder, nil for none.
func coverFor(dir string) []byte {
	covers.RLock()
	defer covers.RUnlock()
	return covers.byFolder[coverKey(dir)]
}
