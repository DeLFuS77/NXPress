package system

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrDeclined is the answer No to the question of Windows for the administrator.
var ErrDeclined = errors.New("the permission of the administrator was not given")

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	window     uintptr
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instance   uintptr
	idList     uintptr
	class      *uint16
	classKey   uintptr
	hotKey     uint32
	icon       uintptr
	process    windows.Handle
}

// RunElevated runs a program as the administrator, which Windows asks the user for, waits for
// its end and gives its exit code: for an update of NXPress in Program Files. Each argument is
// given in quotes.
func RunElevated(program string, args ...string) (int, error) {
	parameters := ""
	for _, arg := range args {
		parameters += syscall.EscapeArg(arg) + " "
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, err := syscall.UTF16PtrFromString(program)
	if err != nil {
		return 0, err
	}
	params, err := syscall.UTF16PtrFromString(parameters)
	if err != nil {
		return 0, err
	}
	const noCloseProcess, noAsync, flagNoUI = 0x40, 0x100, 0x400
	info := shellExecuteInfo{mask: noCloseProcess | noAsync | flagNoUI, verb: verb, file: file, parameters: params}
	info.size = uint32(unsafe.Sizeof(info))
	ok, _, callErr := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return 0, ErrDeclined
		}
		return 0, fmt.Errorf("%s cannot be started as the administrator: %v", program, callErr)
	}
	if info.process == 0 {
		return 0, fmt.Errorf("%s started as the administrator gave no process", program)
	}
	defer windows.CloseHandle(info.process)
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return 0, err
	}
	return int(code), nil
}
