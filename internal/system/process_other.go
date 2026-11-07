//go:build !windows

package system

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

// WaitForExit waits for a process to stop, a while at most.
func WaitForExit(pid int, most time.Duration) {
	for deadline := time.Now().Add(most); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
	}
}

// StartDetached starts a program that goes on by itself, in a session of its own: the new
// NXPress after an update (#91).
func StartDetached(program string, args ...string) error {
	command := exec.Command(program, args...)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
