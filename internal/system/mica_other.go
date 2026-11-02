//go:build !windows

package system

// MicaWindow: Mica is only on Windows 11.
func MicaWindow() bool { return false }
