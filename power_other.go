//go:build !windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

// keepAwake keeps the computer from going to sleep while files are at work, until the
// function it gives is called: caffeinate on macOS, systemd-inhibit on Linux, when there.
func keepAwake() func() {
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.Command("caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	} else {
		command = exec.Command("systemd-inhibit", "--what=sleep:idle", "--who=NXPress", "--why=Files at work", "--mode=block", "sleep", "infinity")
	}
	if command.Start() != nil {
		return func() {}
	}
	return func() {
		command.Process.Kill()
		command.Wait()
	}
}

// shutDown turns the computer off.
func shutDown() error {
	if runtime.GOOS == "darwin" {
		return exec.Command("osascript", "-e", `tell application "System Events" to shut down`).Run()
	}
	return exec.Command("systemctl", "poweroff").Run()
}
