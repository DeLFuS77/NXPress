package main

import (
	"os/exec"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

// hideConsole closes the console Windows opens for NXPress when it is started from the
// Explorer: then NXPress is the only program in it. Started from a terminal, it is kept.
func hideConsole() {
	var processes [2]uint32
	count, _, _ := kernel32.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&processes[0])), 2)
	if count != 1 {
		return
	}
	if console, _, _ := kernel32.NewProc("GetConsoleWindow").Call(); console != 0 {
		syscall.NewLazyDLL("user32.dll").NewProc("ShowWindow").Call(console, 0)
	}
	kernel32.NewProc("FreeConsole").Call()
}

// showFile opens the folder of a file in the Explorer, the file chosen.
func showFile(path string) {
	command := exec.Command("explorer.exe")
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	command.Start()
}
