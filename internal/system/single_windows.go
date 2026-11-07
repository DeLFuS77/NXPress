//go:build windows

package system

import (
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// One window of NXPress (#87, #88): the files the Explorer gives to an NXPress started while
// another is open go to that one, by the lock of Wails (options.SingleInstanceLock). Its names
// are the ones Wails gives them (internal/frontend/desktop/windows/single_instance.go).
func singleNames(id string) (mutex, class, window string) {
	id = "wails-app-" + id
	return id + "sim", id + "-sic", id + "-siw"
}

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procShowWindow               = user32.NewProc("ShowWindow")
	procCreateSemaphoreW         = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateSemaphoreW")
	procReleaseSemaphore         = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReleaseSemaphore")
)

// StartWindow lets one NXPress at a time start its window. The Explorer starts an NXPress for
// each file chosen, all at once; the lock of Wails sends the files of each to the window open,
// but one that comes while the first is between making its lock and the window that takes the
// files opens a window of its own. So each waits its turn: the first keeps it until its window
// takes files (done, from OnStartup), and the ones after find that window and give the turn
// back at once. It tells whether another NXPress has a window. A turn kept by an NXPress that
// ended without giving it back is taken all the same after a while.
func StartWindow(id string) (done func(), another bool) {
	name, _ := windows.UTF16PtrFromString("nxpress-start-" + id)
	turn, _, _ := procCreateSemaphoreW.Call(0, 1, 1, uintptr(unsafe.Pointer(name)))
	if turn == 0 {
		return func() {}, WaitForFirstWindow(id)
	}
	windows.WaitForSingleObject(windows.Handle(turn), 15000)
	var once sync.Once
	done = func() { once.Do(func() { procReleaseSemaphore.Call(turn, 1, 0) }) }
	if WaitForFirstWindow(id) {
		done()
		return done, true
	}
	return done, false
}

// WaitForFirstWindow waits, a few seconds at most, while an NXPress holds the lock but cannot
// take files yet. The Explorer starts an NXPress for each file chosen, all at once: the ones
// after the first find the lock before the first has made the window that takes them, and
// Wails then opens a window for each.
// It tells whether there is one.
func WaitForFirstWindow(id string) bool {
	mutex, class, window := singleNames(id)
	name, _ := windows.UTF16PtrFromString(mutex)
	handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	windows.CloseHandle(handle)
	className, _ := windows.UTF16PtrFromString(class)
	windowName, _ := windows.UTF16PtrFromString(window)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if found, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowName))); found != 0 {
			break
		}
	}
	return true
}

// AllowForeground lets the NXPress the files go to come to the front: a program can do it only
// when the one the user started allows it.
func AllowForeground() {
	const anyProcess = ^uintptr(0)
	procAllowSetForegroundWindow.Call(anyProcess)
}

// BringToFront puts the window of NXPress in front of the others, restored when it was
// minimized; not one not shown yet, which comes in front as it shows.
func BringToFront() {
	window := FindOwnWindow(false)
	if window == 0 {
		return
	}
	if iconic, _, _ := procIsIconic.Call(uintptr(window)); iconic != 0 {
		const restore = 9
		procShowWindow.Call(uintptr(window), restore)
	}
	procSetForegroundWindow.Call(uintptr(window))
}
