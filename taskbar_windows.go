//go:build windows

package main

import (
	"runtime"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// the states of the progress on the button of the taskbar (ITaskbarList3::SetProgressState)
const (
	taskbarNone   = 0x0
	taskbarNormal = 0x2
	taskbarError  = 0x4
	taskbarPaused = 0x8
)

var (
	clsidTaskbarList = ole.NewGUID("{56FDF344-FD6D-11d0-958A-006097C9A090}")
	iidTaskbarList3  = ole.NewGUID("{EA1AFB91-9E28-4B86-90E9-9E9F8A5EEFAF}")
	taskbarCalls     = make(chan func(*ole.IUnknown, windows.HWND), 16)
)

// setTaskbarProgress shows the progress of the work on the button of NXPress in the taskbar:
// state is "none", "normal", "paused" or "error", done of total (#31). The calls go to one
// thread of their own, where COM lives.
func setTaskbarProgress(state string, done uint64, total uint64) {
	flag := map[string]uintptr{"normal": taskbarNormal, "paused": taskbarPaused, "error": taskbarError}[state]
	call := func(list *ole.IUnknown, window windows.HWND) {
		methods := *(*[11]uintptr)(unsafe.Pointer(list.RawVTable))
		// SetProgressState is the method 10 of the table, SetProgressValue the 9
		syscall.SyscallN(methods[10], uintptr(unsafe.Pointer(list)), uintptr(window), flag)
		if flag == taskbarNormal || flag == taskbarPaused || flag == taskbarError {
			syscall.SyscallN(methods[9], uintptr(unsafe.Pointer(list)), uintptr(window), uintptr(done), uintptr(total))
		}
	}
	select {
	case taskbarCalls <- call:
	default:
		// a state more recent comes soon: one left out does no harm
	}
}

func init() {
	go func() {
		runtime.LockOSThread()
		// an error here is S_FALSE, COM already initialized on this thread: it works the same
		ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		var list *ole.IUnknown
		var window windows.HWND
		for call := range taskbarCalls {
			if window == 0 || !windows.IsWindowVisible(window) {
				window = ownWindow()
			}
			if window == 0 {
				continue
			}
			if list == nil {
				created, err := ole.CreateInstance(clsidTaskbarList, iidTaskbarList3)
				if err != nil {
					continue
				}
				methods := *(*[4]uintptr)(unsafe.Pointer(created.RawVTable))
				// HrInit, the method 3
				syscall.SyscallN(methods[3], uintptr(unsafe.Pointer(created)))
				list = created
			}
			call(list, window)
		}
	}()
}

// the window found by ownWindow; the callback is made once, as Windows takes a few only
var (
	foundWindow windows.HWND
	findWindow  = syscall.NewCallback(func(window windows.HWND, _ uintptr) uintptr {
		var owner uint32
		windows.GetWindowThreadProcessId(window, &owner)
		if owner != windows.GetCurrentProcessId() || !windows.IsWindowVisible(window) {
			return 1
		}
		title := make([]uint16, 64)
		length, _ := getWindowText(window, &title[0], int32(len(title)))
		if name := windows.UTF16ToString(title[:max(length, 0)]); len(name) >= 7 && name[:7] == "NXPress" {
			foundWindow = window
			return 0
		}
		return 1
	})
)

// ownWindow is the main window of this process: the visible one titled NXPress.
func ownWindow() windows.HWND {
	foundWindow = 0
	windows.EnumWindows(findWindow, nil)
	return foundWindow
}

var procGetWindowTextW = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowTextW")

func getWindowText(window windows.HWND, text *uint16, size int32) (int32, error) {
	length, _, err := procGetWindowTextW.Call(uintptr(window), uintptr(unsafe.Pointer(text)), uintptr(size))
	return int32(length), err
}
