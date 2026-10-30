//go:build !windows

package main

// micaWindow: Mica is only on Windows 11.
func micaWindow() bool { return false }
