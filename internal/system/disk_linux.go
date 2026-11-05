package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// SeekPenalty tells whether the drive of a path loses time going from one place to another, as a
// hard disk does and an SSD does not, as Linux tells it (rotational); known is false when it
// cannot tell.
func SeekPenalty(path string) (penalty bool, known bool) {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil {
		return false, false
	}
	device := fmt.Sprintf("/sys/dev/block/%d:%d", unix.Major(uint64(stat.Dev)), unix.Minor(uint64(stat.Dev)))
	// a partition has no queue of its own: the one of its disk is the folder above it
	for _, candidate := range []string{filepath.Join(device, "queue", "rotational"), filepath.Join(device, "..", "queue", "rotational")} {
		if data, err := os.ReadFile(candidate); err == nil {
			return strings.TrimSpace(string(data)) == "1", true
		}
	}
	return false, false
}
