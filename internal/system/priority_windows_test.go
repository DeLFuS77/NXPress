package system

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Low priority puts the process below the normal class with its memory low, and back.
func TestSetLowPriority(t *testing.T) {
	process := windows.CurrentProcess()
	class := func() uint32 {
		value, _ := windows.GetPriorityClass(process)
		return value
	}
	memory := func() uint32 {
		var value uint32
		kernel32.NewProc("GetProcessInformation").Call(uintptr(process), processMemoryPriority, uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value))
		return value
	}
	defer SetLowPriority(false)
	if err := SetLowPriority(true); err != nil || class() != belowNormalPriorityClass || memory() != memoryPriorityLow {
		t.Fatalf("low: class %#x, memory %d, %v", class(), memory(), err)
	}
	if err := SetLowPriority(false); err != nil || class() != normalPriorityClass || memory() != memoryPriorityNormal {
		t.Fatalf("back: class %#x, memory %d, %v", class(), memory(), err)
	}
}

// The processes started go below the normal priority, those they start too, and each comes back
// to the class it had.
func TestSetChildrenLowPriority(t *testing.T) {
	// cmd starts ping, which waits: a child and a child of it
	command := exec.Command("cmd.exe", "/c", "ping -n 30 127.0.0.1 >nul")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.ABOVE_NORMAL_PRIORITY_CLASS}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	classes := func() map[uint32]uint32 {
		found := map[uint32]uint32{}
		for c, process := range openChildren() {
			found[c.id], _ = windows.GetPriorityClass(process)
			windows.CloseHandle(process)
		}
		return found
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(classes()) < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	before := classes()
	if len(before) < 2 || before[uint32(command.Process.Pid)] != windows.ABOVE_NORMAL_PRIORITY_CLASS {
		t.Fatalf("the child and its child were not found: %v", before)
	}

	SetChildrenLowPriority(true)
	for id, class := range classes() {
		if class != belowNormalPriorityClass {
			t.Fatalf("process %d low: class %#x", id, class)
		}
	}
	SetChildrenLowPriority(false)
	for id, class := range classes() {
		if class != before[id] {
			t.Fatalf("process %d back: class %#x, want %#x", id, class, before[id])
		}
	}
}
