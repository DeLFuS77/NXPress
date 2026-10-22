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

// keepAwake keeps the computer from going to sleep while files are at work, until the
// function it gives is called.
func keepAwake() func() {
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

// shutDown turns the computer off.
func shutDown() error {
	command := exec.Command("shutdown.exe", "/s", "/t", "0")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Run()
}
