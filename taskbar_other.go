//go:build !windows

package main

// setTaskbarProgress shows the progress of the work on the button of the taskbar: on Windows
// only (#31); macOS and Linux show none.
func setTaskbarProgress(state string, done uint64, total uint64) {}
