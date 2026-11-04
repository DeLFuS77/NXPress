//go:build !windows

package system

import (
	"os"
	"runtime"
	"strconv"
	"syscall"
)

// SetLowPriority runs NXPress at nice 10, so the rest of the computer stays responsive while it
// works; false gives back nice 0. On Linux each thread has a nice value of its own, and a new
// one takes the value of the thread that makes it: every thread of the process is set. Linux
// lets a user raise it back only within the limit of the user (RLIMIT_NICE, usually none): when
// that fails, NXPress stays low until it is opened again.
func SetLowPriority(low bool) error {
	nice := 0
	if low {
		nice = 10
	}
	if runtime.GOOS == "linux" {
		if tasks, err := os.ReadDir("/proc/self/task"); err == nil {
			var first error
			for _, task := range tasks {
				id, err := strconv.Atoi(task.Name())
				if err != nil {
					continue
				}
				if err := syscall.Setpriority(syscall.PRIO_PROCESS, id, nice); err != nil && first == nil {
					first = err
				}
			}
			return first
		}
	}
	return syscall.Setpriority(syscall.PRIO_PROCESS, 0, nice)
}

// SetChildrenLowPriority does nothing here: the processes of the window of WebKit keep their own
// priority on Linux and macOS, where they take little of the processor (on macOS they are not
// even started by NXPress).
func SetChildrenLowPriority(low bool) {}
