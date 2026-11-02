package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/DeLFuS77/NXPress/internal/system"
	"github.com/DeLFuS77/NXPress/internal/windowplace"
	"golang.org/x/sys/windows"
)

// The thread of the taskbar always ends on the last state asked for, however many come while
// it shows one, and never goes back to an older one.
func TestTaskbarKeepsTheLastState(t *testing.T) {
	slot := system.NewTaskbarSlot()
	const last = 2000
	go func() {
		for done := uint64(1); done <= last; done++ {
			slot.Put(system.TaskbarState{Flag: system.TaskbarNormal, Done: done, Total: last})
		}
	}()
	shown := uint64(0)
	deadline := time.Now().Add(5 * time.Second)
	for shown < last {
		if time.Now().After(deadline) {
			t.Fatalf("the last state shown is %d of %d", shown, last)
		}
		state := slot.Take()
		if state.Done < shown {
			t.Fatalf("%d shown after %d", state.Done, shown)
		}
		shown = state.Done
		time.Sleep(time.Millisecond)
	}
}

// taskbarNow is the last state asked for the taskbar.
func taskbarNow() system.TaskbarState {
	system.Taskbar.Mutex.Lock()
	defer system.Taskbar.Mutex.Unlock()
	return system.Taskbar.State
}

// The taskbar shows progress only while there is work, and ends as the work did: nothing once
// stopped, red with files that failed.
func TestTaskbarFollowsTheWork(t *testing.T) {
	catchEvents(t)
	a := &App{ctx: context.Background()}
	system.SetTaskbarProgress("none", 7, 7)
	a.SetTaskbar("normal", 3, 9)
	if now := taskbarNow(); now != (system.TaskbarState{Flag: system.TaskbarNone, Done: 7, Total: 7}) {
		t.Fatalf("progress without work is shown: %+v", now)
	}

	if err := a.Start(Request{Mode: "recompress", Files: []string{t.TempDir()}, Watch: true}); err != nil {
		t.Fatal(err)
	}
	a.SetTaskbar("paused", 3, 9)
	if now := taskbarNow(); now != (system.TaskbarState{Flag: system.TaskbarPaused, Done: 3, Total: 9}) {
		t.Fatalf("progress at work is not shown: %+v", now)
	}
	a.closed(context.Background())
	if now := taskbarNow(); now.Flag != system.TaskbarNone {
		t.Fatalf("stopped, the taskbar shows %+v", now)
	}
	a.SetTaskbar("normal", 9, 9)
	if now := taskbarNow(); now.Flag != system.TaskbarNone {
		t.Fatalf("progress told late is shown: %+v", now)
	}

	folder := t.TempDir()
	os.WriteFile(filepath.Join(folder, "broken.nsz"), []byte("not a game at all"), 0o644)
	if err := a.Start(Request{Mode: "recompress", Files: []string{folder}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the work did not end", func() bool { return !a.working() })
	if now := taskbarNow(); now != (system.TaskbarState{Flag: system.TaskbarError, Done: 1, Total: 1}) {
		t.Fatalf("with a file failed, the taskbar shows %+v", now)
	}
}

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
	if found := system.FindOwnWindow(true); found != window {
		t.Fatalf("the hidden window is not found: %v, want %v", found, window)
	}
	if system.OwnWindow() != 0 {
		t.Fatal("a hidden window is taken for the one shown")
	}

	left := windowplace.WindowRect{Left: 120, Top: 80, Right: 1020, Bottom: 730}
	if !windowplace.RestorePlacement(left) {
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
		place, maximised, ok := windowplace.OwnPlacement()
		if !ok || maximised || place != left {
			t.Fatalf("the place read back is %+v (maximised %v, %v), want %+v", place, maximised, ok, left)
		}
		windowplace.RestorePlacement(place)
	}
}

// The window is no bigger than the work area of its screen.
func TestWindowPlaceFitsTheScreen(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hiddenWindow(t)
	if !windowplace.RestorePlacement(windowplace.WindowRect{Left: 40, Top: 30, Right: 40 + 100000, Bottom: 30 + 100000}) {
		t.Fatal("the place is not put back")
	}
	place, _, ok := windowplace.OwnPlacement()
	if !ok {
		t.Fatal("the place is not read")
	}
	normal := windows.Rect{Left: place.Left, Top: place.Top, Right: place.Right, Bottom: place.Bottom}
	screen, _, _ := windowplace.ProcMonitorFromRect.Call(uintptr(unsafe.Pointer(&normal)), windowplace.MonitorDefaultToNearest)
	info := windowplace.MonitorInfo{Size: uint32(unsafe.Sizeof(windowplace.MonitorInfo{}))}
	windowplace.ProcGetMonitorInfoW.Call(screen, uintptr(unsafe.Pointer(&info)))
	width, height := place.Right-place.Left, place.Bottom-place.Top
	if width <= 0 || height <= 0 || width > info.Work.Right-info.Work.Left || height > info.Work.Bottom-info.Work.Top {
		t.Fatalf("the window is %dx%d, the work area %dx%d", width, height, info.Work.Right-info.Work.Left, info.Work.Bottom-info.Work.Top)
	}
}
