package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// sendToFolder is the folder of the "Send to" menu of the Explorer, for this user.
func sendToFolder() string {
	if data := os.Getenv("APPDATA"); data != "" {
		return filepath.Join(data, "Microsoft", "Windows", "SendTo")
	}
	return ""
}

// sendToShortcut is the shortcut to NXPress in the "Send to" menu.
func sendToShortcut() string {
	return filepath.Join(sendToFolder(), "NXPress.lnk")
}

// sendToEnabled tells whether the "Send to" menu holds NXPress.
func sendToEnabled() bool {
	_, err := os.Stat(sendToShortcut())
	return err == nil
}

// enableSendTo puts a shortcut to NXPress in the "Send to" menu: the files chosen are sent
// to it all at once, and its window opens with them.
func enableSendTo() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return makeShortcut(sendToShortcut(), executable)
}

// disableSendTo takes the shortcut away.
func disableSendTo() error {
	if err := os.Remove(sendToShortcut()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// makeShortcut writes a shortcut at path to target, with the shell of Windows.
func makeShortcut(path string, target string) error {
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	script := "$s = (New-Object -ComObject WScript.Shell).CreateShortcut(" + quote(path) + "); $s.TargetPath = " + quote(target) +
		"; $s.IconLocation = " + quote(target+",0") + "; $s.Description = 'NXPress'; $s.Save()"
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("the shortcut cannot be made: %v %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
