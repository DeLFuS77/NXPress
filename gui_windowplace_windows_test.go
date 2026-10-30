package main

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// hiddenWindow makes a hidden window like the one of NXPress (of the class of Wails, titled
// NXPress), never shown; the test runs on the thread that made it.
func hiddenWindow(t *testing.T) windows.HWND {
	t.Helper()
	user32 := windows.NewLazySystemDLL("user32.dll")
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	class, _ := windows.UTF16PtrFromString("wailsWindow")
	title, _ := windows.UTF16PtrFromString("NXPress test")
	type windowClass struct {
		size, style                  uint32
		procedure                    uintptr
		classExtra, windowExtra      int32
		instance, icon, cursor, back uintptr
		menu, name                   *uint16
		smallIcon                    uintptr
	}
	register := windowClass{procedure: user32.NewProc("DefWindowProcW").Addr(), instance: instance, name: class}
	register.size = uint32(unsafe.Sizeof(register))
	user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&register)))
	const overlappedWindow = 0x00CF0000
	window, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), overlappedWindow, 200, 150, 1000, 700, 0, 0, instance, 0)
	if window == 0 {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		user32.NewProc("DestroyWindow").Call(window)
		user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(class)), instance)
	})
	return windows.HWND(window)
}

// On Windows the window goes back where Windows itself said it was, wherever the taskbar is: read
// and put back, the place does not move. It is found before it is shown, and stays hidden.
func TestWindowPlaceGoesBackWhereItWas(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	window := hiddenWindow(t)
	if found := findOwnWindow(true); found != window {
		t.Fatalf("the hidden window is not found: %v, want %v", found, window)
	}
	if ownWindow() != 0 {
		t.Fatal("a hidden window is taken for the one shown")
	}

	left := windowRect{Left: 120, Top: 80, Right: 1020, Bottom: 730}
	if !restorePlacement(left) {
		t.Fatal("the place is not put back")
	}
	if windows.IsWindowVisible(window) {
		t.Fatal("the window shows before it is placed")
	}
	// hidden, it already has the size it shows with
	var now windows.Rect
	windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowRect").Call(uintptr(window), uintptr(unsafe.Pointer(&now)))
	if now.Right-now.Left != left.Right-left.Left || now.Bottom-now.Top != left.Bottom-left.Top {
		t.Fatalf("the hidden window is %+v, want the size of %+v", now, left)
	}
	for range 3 {
		place, maximised, ok := ownPlacement()
		if !ok || maximised || place != left {
			t.Fatalf("the place read back is %+v (maximised %v, %v), want %+v", place, maximised, ok, left)
		}
		restorePlacement(place)
	}
}

// The window is no bigger than the work area of its screen.
func TestWindowPlaceFitsTheScreen(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hiddenWindow(t)
	if !restorePlacement(windowRect{Left: 40, Top: 30, Right: 40 + 100000, Bottom: 30 + 100000}) {
		t.Fatal("the place is not put back")
	}
	place, _, ok := ownPlacement()
	if !ok {
		t.Fatal("the place is not read")
	}
	normal := windows.Rect{Left: place.Left, Top: place.Top, Right: place.Right, Bottom: place.Bottom}
	screen, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&normal)), monitorDefaultToNearest)
	info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	procGetMonitorInfoW.Call(screen, uintptr(unsafe.Pointer(&info)))
	width, height := place.Right-place.Left, place.Bottom-place.Top
	if width <= 0 || height <= 0 || width > info.work.Right-info.work.Left || height > info.work.Bottom-info.work.Top {
		t.Fatalf("the window is %dx%d, the work area %dx%d", width, height, info.work.Right-info.work.Left, info.work.Bottom-info.work.Top)
	}
}
