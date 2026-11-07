//go:build windows

package system

import (
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// WaitForExit waits for a process to stop, a while at most.
func WaitForExit(pid int, most time.Duration) {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(process)
	windows.WaitForSingleObject(process, uint32(most.Milliseconds()))
}

// StartDetached starts a program that goes on by itself, without a console of its own: the new
// NXPress after an update (#91).
func StartDetached(program string, args ...string) error {
	const createNoWindow = 0x08000000
	command := exec.Command(program, args...)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow | syscall.CREATE_NEW_PROCESS_GROUP}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
