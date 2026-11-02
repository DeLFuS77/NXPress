package system

import "testing"

// Low priority puts the process below the normal class, and back.
func TestSetLowPriority(t *testing.T) {
	class := func() uintptr {
		process, _, _ := kernel32.NewProc("GetCurrentProcess").Call()
		value, _, _ := kernel32.NewProc("GetPriorityClass").Call(process)
		return value
	}
	defer SetLowPriority(false)
	if err := SetLowPriority(true); err != nil || class() != belowNormalPriorityClass {
		t.Fatalf("low: %#x %v", class(), err)
	}
	if err := SetLowPriority(false); err != nil || class() != normalPriorityClass {
		t.Fatalf("back: %#x %v", class(), err)
	}
}
