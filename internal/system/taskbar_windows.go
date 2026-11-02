//go:build windows

package system

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// the states of the progress on the button of the taskbar (ITaskbarList3::SetProgressState)
const (
	TaskbarNone   = 0x0
	TaskbarNormal = 0x2
	TaskbarError  = 0x4
	TaskbarPaused = 0x8
)

var (
	clsidTaskbarList = ole.NewGUID("{56FDF344-FD6D-11d0-958A-006097C9A090}")
	iidTaskbarList3  = ole.NewGUID("{EA1AFB91-9E28-4B86-90E9-9E9F8A5EEFAF}")
	Taskbar          = NewTaskbarSlot()
)

// TaskbarState is a state of the progress on the button of the taskbar, done of total.
type TaskbarState struct {
	Flag        uintptr
	Done, Total uint64
}

// taskbarSlot keeps the last state asked for until the thread of the taskbar takes it: a state
// asked for meanwhile takes the place of the one not shown yet, so the last one is always shown.
type taskbarSlot struct {
	Mutex sync.Mutex
	State TaskbarState
	wake  chan struct{}
}

func NewTaskbarSlot() *taskbarSlot {
	return &taskbarSlot{wake: make(chan struct{}, 1)}
}

func (s *taskbarSlot) Put(state TaskbarState) {
	s.Mutex.Lock()
	s.State = state
	s.Mutex.Unlock()
	// one wake waiting is enough: it takes the last state
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Take waits for a state put, and gives the last one.
func (s *taskbarSlot) Take() TaskbarState {
	<-s.wake
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	return s.State
}

// SetTaskbarProgress shows the progress of the work on the button of NXPress in the taskbar:
// state is "none", "normal", "paused" or "error", done of total (#31). The states go to one
// thread of their own, where COM lives.
func SetTaskbarProgress(state string, done uint64, total uint64) {
	flag := map[string]uintptr{"normal": TaskbarNormal, "paused": TaskbarPaused, "error": TaskbarError}[state]
	Taskbar.Put(TaskbarState{Flag: flag, Done: done, Total: total})
}

// showTaskbarState shows a state on the button of the window.
func showTaskbarState(list *ole.IUnknown, window windows.HWND, state TaskbarState) {
	methods := *(*[11]uintptr)(unsafe.Pointer(list.RawVTable))
	// SetProgressState is the method 10 of the table, SetProgressValue the 9
	syscall.SyscallN(methods[10], uintptr(unsafe.Pointer(list)), uintptr(window), state.Flag)
	if state.Flag == TaskbarNormal || state.Flag == TaskbarPaused || state.Flag == TaskbarError {
		syscall.SyscallN(methods[9], uintptr(unsafe.Pointer(list)), uintptr(window), uintptr(state.Done), uintptr(state.Total))
	}
}

func init() {
	go func() {
		runtime.LockOSThread()
		// an error here is S_FALSE, COM already initialized on this thread: it works the same
		ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		var list *ole.IUnknown
		var window windows.HWND
		for {
			state := Taskbar.Take()
			if window == 0 || !windows.IsWindowVisible(window) {
				window = OwnWindow()
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
			showTaskbarState(list, window, state)
		}
	}()
}

// the window found by findOwnWindow, and whether a hidden one is looked for; the callback is made
// once, as Windows takes a few only
var (
	findMutex   sync.Mutex
	foundWindow windows.HWND
	findHidden  bool
	findWindow  = syscall.NewCallback(func(window windows.HWND, _ uintptr) uintptr {
		var owner uint32
		windows.GetWindowThreadProcessId(window, &owner)
		if owner != windows.GetCurrentProcessId() || !findHidden && !windows.IsWindowVisible(window) {
			return 1
		}
		// the window of Wails, not a message titled NXPress
		class := make([]uint16, 64)
		if length, _ := windows.GetClassName(window, &class[0], int32(len(class))); windows.UTF16ToString(class[:max(length, 0)]) != "wailsWindow" {
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

// OwnWindow is the main window of this process: the visible one titled NXPress.
func OwnWindow() windows.HWND {
	return FindOwnWindow(false)
}

// FindOwnWindow is the main window of this process, titled NXPress; with hidden, also before it
// is shown.
func FindOwnWindow(hidden bool) windows.HWND {
	findMutex.Lock()
	defer findMutex.Unlock()
	foundWindow, findHidden = 0, hidden
	windows.EnumWindows(findWindow, nil)
	return foundWindow
}

var procGetWindowTextW = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowTextW")

func getWindowText(window windows.HWND, text *uint16, size int32) (int32, error) {
	length, _, err := procGetWindowTextW.Call(uintptr(window), uintptr(unsafe.Pointer(text)), uintptr(size))
	return int32(length), err
}
