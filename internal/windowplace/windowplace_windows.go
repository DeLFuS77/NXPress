//go:build windows

package windowplace

import (
	"unsafe"

	"github.com/DeLFuS77/NXPress/internal/system"

	"golang.org/x/sys/windows"
)

var (
	procGetWindowPlacement = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowPlacement")
	procSetWindowPlacement = windows.NewLazySystemDLL("user32.dll").NewProc("SetWindowPlacement")
	ProcMonitorFromRect    = windows.NewLazySystemDLL("user32.dll").NewProc("MonitorFromRect")
	ProcGetMonitorInfoW    = windows.NewLazySystemDLL("user32.dll").NewProc("GetMonitorInfoW")
)

// windowPlacement is the WINDOWPLACEMENT of Windows: where a window goes when it is neither
// maximised nor minimised, in the coordinates of the work area (the screens without the
// taskbar), and how it shows.
type windowPlacement struct {
	length                   uint32
	flags                    uint32
	showCmd                  uint32
	minimisedAt, maximisedAt struct{ x, y int32 }
	normal                   windows.Rect
}

// MonitorInfo is the MONITORINFO of Windows: a screen, and its work area.
type MonitorInfo struct {
	Size   uint32
	screen windows.Rect
	Work   windows.Rect
	flags  uint32
}

const (
	swHide                  = 0
	swShowMinimized         = 2
	swShowMaximized         = 3
	wpfRestoreToMaximized   = 2
	MonitorDefaultToNearest = 2
)

// placement gives the WINDOWPLACEMENT of a window.
func placement(window windows.HWND) (windowPlacement, bool) {
	place := windowPlacement{length: uint32(unsafe.Sizeof(windowPlacement{}))}
	ok, _, _ := procGetWindowPlacement.Call(uintptr(window), uintptr(unsafe.Pointer(&place)))
	return place, ok != 0
}

// OwnPlacement gives where the window of NXPress goes when neither maximised nor minimised, as
// Windows keeps it, and whether it is maximised (or goes back to it from the taskbar). Read and
// put back the same way, the place stays where it was, wherever the taskbar is.
func OwnPlacement() (WindowRect, bool, bool) {
	window := system.FindOwnWindow(true)
	if window == 0 {
		return WindowRect{}, false, false
	}
	place, ok := placement(window)
	if !ok {
		return WindowRect{}, false, false
	}
	maximised := place.showCmd == swShowMaximized || place.showCmd == swShowMinimized && place.flags&wpfRestoreToMaximized != 0
	normal := place.normal
	return WindowRect{Left: normal.Left, Top: normal.Top, Right: normal.Right, Bottom: normal.Bottom}, maximised, true
}

// RestorePlacement puts the window of NXPress, still hidden, where it was left, no bigger than the
// work area of the screen it goes on: Windows brings onto a screen a place no screen holds any
// longer.
func RestorePlacement(rect WindowRect) bool {
	window := system.FindOwnWindow(true)
	if window == 0 || rect.Right <= rect.Left || rect.Bottom <= rect.Top {
		return false
	}
	place, ok := placement(window)
	if !ok {
		return false
	}
	normal := windows.Rect{Left: rect.Left, Top: rect.Top, Right: rect.Right, Bottom: rect.Bottom}
	if screen, _, _ := ProcMonitorFromRect.Call(uintptr(unsafe.Pointer(&normal)), MonitorDefaultToNearest); screen != 0 {
		info := MonitorInfo{Size: uint32(unsafe.Sizeof(MonitorInfo{}))}
		if ok, _, _ := ProcGetMonitorInfoW.Call(screen, uintptr(unsafe.Pointer(&info))); ok != 0 {
			normal.Right = normal.Left + min(normal.Right-normal.Left, info.Work.Right-info.Work.Left)
			normal.Bottom = normal.Top + min(normal.Bottom-normal.Top, info.Work.Bottom-info.Work.Top)
		}
	}
	// hidden still: the window shows once placed, maximised then when it was
	place.flags, place.showCmd, place.normal = 0, swHide, normal
	placed, _, _ := procSetWindowPlacement.Call(uintptr(window), uintptr(unsafe.Pointer(&place)))
	return placed != 0
}
