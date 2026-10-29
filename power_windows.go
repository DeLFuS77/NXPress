package main

import (
	"os/exec"
	"runtime"
	"syscall"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

// preventSleep keeps the computer from going to sleep until the function it gives is called.
func preventSleep() func() {
	state := kernel32.NewProc("SetThreadExecutionState")
	done := make(chan struct{})
	released := make(chan struct{})
	// the state belongs to a thread: one goroutine, locked to its thread, holds it
	go func() {
		runtime.LockOSThread()
		state.Call(esContinuous | esSystemRequired)
		<-done
		state.Call(esContinuous)
		close(released)
	}()
	return func() {
		close(done)
		<-released
	}
}

// shutDown turns the computer off; forced, the programs left open are closed without asking to
// save, which else can keep it on.
func shutDown(force bool) error {
	arguments := []string{"/s", "/t", "0"}
	if force {
		arguments = append(arguments, "/f")
	}
	command := exec.Command("shutdown.exe", arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Run()
}
