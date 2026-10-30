//go:build !windows

package main

// fileBusy tells whether another program has a file open to write: Linux and macOS do not tell
// it, so a file is whole once its size and the time it was last written stay the same.
func fileBusy(path string) bool { return false }
