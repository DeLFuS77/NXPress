//go:build !windows

package system

// FileBusy tells whether another program has a file open to write: Linux and macOS do not tell
// it, so a file is whole once its size and the time it was last written stay the same.
func FileBusy(path string) bool { return false }
