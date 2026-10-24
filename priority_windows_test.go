package main

import "testing"

// Low priority puts the process below the normal class, and back.
func TestSetLowPriority(t *testing.T) {
	class := func() uintptr {
		process, _, _ := kernel32.NewProc("GetCurrentProcess").Call()
		value, _, _ := kernel32.NewProc("GetPriorityClass").Call(process)
		return value
	}
	defer setLowPriority(false)
	if err := setLowPriority(true); err != nil || class() != belowNormalPriorityClass {
		t.Fatalf("low: %#x %v", class(), err)
	}
	if err := setLowPriority(false); err != nil || class() != normalPriorityClass {
		t.Fatalf("back: %#x %v", class(), err)
	}
}
