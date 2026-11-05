package system

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	normalPriorityClass      = 0x00000020
	belowNormalPriorityClass = 0x00004000

	// the memory priority of a process, as SetProcessInformation takes it
	processMemoryPriority = 0
	memoryPriorityLow     = 2
	memoryPriorityNormal  = 5
)

// SetLowPriority runs NXPress below the normal priority of the processor, so the rest of the
// computer stays responsive while it works: the threads of zstd belong to its process and go
// down with it. Its memory goes low too: when memory runs short, the pages of NXPress are the
// first given to the other programs (#70). false gives it back the normal priority.
//
// The efficiency mode of Windows (EcoQoS) is not used: on a processor with efficient cores it
// made a compression six times slower with nothing else running, and the programs in front ran
// as fast without it.
func SetLowPriority(low bool) error {
	class, memory := uint32(normalPriorityClass), uint32(memoryPriorityNormal)
	if low {
		class, memory = belowNormalPriorityClass, memoryPriorityLow
	}
	process := windows.CurrentProcess()
	if err := windows.SetPriorityClass(process, class); err != nil {
		return err
	}
	kernel32.NewProc("SetProcessInformation").Call(uintptr(process), processMemoryPriority, uintptr(unsafe.Pointer(&memory)), unsafe.Sizeof(memory))
	return nil
}

// a process started by NXPress: its number and when it started, as Windows gives the number of a
// process that ended to the next ones
type child struct {
	id      uint32
	started int64
}

var (
	childrenMutex sync.Mutex
	// the processes put below the normal priority, and the class each had
	lowered = map[child]uint32{}
)

// SetChildrenLowPriority puts the processes NXPress started, the WebView2 of its window, below the
// normal priority too (#70), or gives each back the class it had. WebView2 sets theirs again as
// the window shows or hides: while NXPress is low, it is to be called again now and then.
func SetChildrenLowPriority(low bool) {
	childrenMutex.Lock()
	defer childrenMutex.Unlock()
	if !low {
		for c, class := range lowered {
			if process, ok := openChild(c); ok {
				if now, err := windows.GetPriorityClass(process); err == nil && now == belowNormalPriorityClass {
					windows.SetPriorityClass(process, class)
				}
				windows.CloseHandle(process)
			}
		}
		clear(lowered)
		return
	}
	running := openChildren()
	for c, process := range running {
		class, err := windows.GetPriorityClass(process)
		above := class == windows.NORMAL_PRIORITY_CLASS || class == windows.ABOVE_NORMAL_PRIORITY_CLASS || class == windows.HIGH_PRIORITY_CLASS
		if err == nil && above && windows.SetPriorityClass(process, belowNormalPriorityClass) == nil {
			// raised again by WebView2, it gets back the class it had first
			if _, kept := lowered[c]; !kept {
				lowered[c] = class
			}
		}
		windows.CloseHandle(process)
	}
	for c := range lowered {
		if _, ok := running[c]; !ok {
			delete(lowered, c)
		}
	}
}

// openChildren opens the processes NXPress started, and the ones they started, as they run now.
// One whose parent has the number of one of them but which started before it is not one: its
// parent ended and its number was given away. The handles are to be closed.
func openChildren() map[child]windows.Handle {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snapshot)
	started := map[uint32][]uint32{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		started[entry.ParentProcessID] = append(started[entry.ParentProcessID], entry.ProcessID)
	}
	own := windows.GetCurrentProcessId()
	opened := map[child]windows.Handle{}
	parents := []child{{own, startTime(windows.CurrentProcess())}}
	for len(parents) > 0 {
		parent := parents[0]
		parents = parents[1:]
		for _, id := range started[parent.id] {
			if id == own {
				continue
			}
			process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_SET_INFORMATION, false, id)
			if err != nil {
				continue
			}
			c := child{id, startTime(process)}
			if _, seen := opened[c]; seen || c.started < parent.started {
				windows.CloseHandle(process)
				continue
			}
			opened[c] = process
			parents = append(parents, c)
		}
	}
	return opened
}

// openChild opens a process lowered before, if it still runs; the handle is to be closed.
func openChild(c child) (windows.Handle, bool) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_SET_INFORMATION, false, c.id)
	if err != nil {
		return 0, false
	}
	if startTime(process) != c.started {
		windows.CloseHandle(process)
		return 0, false
	}
	return process, true
}

// startTime is when a process started, or 0 when Windows does not tell.
func startTime(process windows.Handle) int64 {
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(process, &created, &exited, &kernel, &user) != nil {
		return 0
	}
	return created.Nanoseconds()
}
