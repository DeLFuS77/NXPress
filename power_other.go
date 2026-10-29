//go:build !windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

// keepAwake keeps the computer from going to sleep while files are at work, until the
// function it gives is called: caffeinate on macOS, systemd-inhibit on Linux, when there. Both
// end with NXPress too, if it ends without calling it: caffeinate waits for it, and on Linux
// tail, which systemd-inhibit holds the computer awake for.
func keepAwake() func() {
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.Command("caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	} else {
		command = exec.Command("systemd-inhibit", "--what=sleep:idle", "--who=NXPress", "--why=Files at work", "--mode=block", "tail", "--pid="+strconv.Itoa(os.Getpid()), "-f", "/dev/null")
	}
	if command.Start() != nil {
		return func() {}
	}
	return func() {
		command.Process.Kill()
		command.Wait()
	}
}

// shutDown turns the computer off; forced, on Linux the programs that hold it on are not waited
// for (macOS has no such choice: it asks them).
func shutDown(force bool) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("osascript", "-e", `tell application "System Events" to shut down`).Run()
	}
	if force {
		return exec.Command("systemctl", "poweroff", "-i").Run()
	}
	return exec.Command("systemctl", "poweroff").Run()
}
