package main

const (
	normalPriorityClass      = 0x00000020
	belowNormalPriorityClass = 0x00004000
)

// setLowPriority runs NXPress below the normal priority of the processor, so the rest of the
// computer stays responsive while it works: the threads of zstd belong to its process and go
// down with it. false gives it back the normal priority.
func setLowPriority(low bool) error {
	class := uintptr(normalPriorityClass)
	if low {
		class = belowNormalPriorityClass
	}
	process, _, _ := kernel32.NewProc("GetCurrentProcess").Call()
	if ok, _, err := kernel32.NewProc("SetPriorityClass").Call(process, class); ok == 0 {
		return err
	}
	return nil
}
