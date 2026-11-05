//go:build !windows && !linux

package system

// SeekPenalty tells whether the drive of a path loses time going from one place to another;
// known is false where the system is not asked.
func SeekPenalty(path string) (penalty bool, known bool) {
	return false, false
}
