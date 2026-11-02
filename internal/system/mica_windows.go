//go:build windows

package system

import "golang.org/x/sys/windows"

// MicaWindow tells whether the window can show the Mica of Windows 11 behind its menu: from
// Windows 11 22H2 (build 22621), where the backdrop of a window can be chosen.
func MicaWindow() bool {
	return windows.RtlGetVersion().BuildNumber >= 22621
}
