package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
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

// sendToEnabled tells whether the "Send to" menu holds NXPress: its shortcut, to this program. A
// shortcut to another copy of it (an older one, moved or deleted since) does not count, so
// turning the menu on writes it again, to this one.
func sendToEnabled() bool {
	shortcut := sendToShortcut()
	if _, err := os.Stat(shortcut); err != nil {
		return false
	}
	executable, err := os.Executable()
	if err != nil {
		return true
	}
	return shortcutTo(shortcut, executable)
}

// shortcutTo tells whether a shortcut points to target. One that cannot be read is taken as it
// is: only a path read, and another, says no.
func shortcutTo(shortcut string, target string) bool {
	path, err := shortcutTarget(shortcut)
	if err != nil {
		return true
	}
	if strings.EqualFold(filepath.Clean(path), filepath.Clean(target)) {
		return true
	}
	// the same file by another name: a short name of a folder, a link
	pathInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	targetInfo, err := os.Stat(target)
	return err == nil && os.SameFile(pathInfo, targetInfo)
}

// shortcutTarget reads the local path a shortcut (.lnk) points to, from its LinkInfo (the Shell
// Link format of Windows): in UTF-16 when the shortcut holds it so, else in the code page of the
// system. A shortcut to a network folder has none.
func shortcutTarget(shortcut string) (string, error) {
	data, err := os.ReadFile(shortcut)
	if err != nil {
		return "", err
	}
	const (
		headerSize  = 0x4C
		hasIDList   = 1 << 0
		hasLinkInfo = 1 << 1
		// of the LinkInfo: it has a volume and a local path
		localPath = 1 << 0
	)
	notRead := errors.New("the shortcut holds no local path")
	if len(data) < headerSize || binary.LittleEndian.Uint32(data) != headerSize {
		return "", notRead
	}
	flags := binary.LittleEndian.Uint32(data[0x14:])
	at := headerSize
	if flags&hasIDList != 0 {
		if len(data) < at+2 {
			return "", notRead
		}
		at += 2 + int(binary.LittleEndian.Uint16(data[at:]))
	}
	if flags&hasLinkInfo == 0 || len(data) < at+0x1C {
		return "", notRead
	}
	info := data[at:]
	size := int(binary.LittleEndian.Uint32(info))
	if size < 0x1C || size > len(info) || binary.LittleEndian.Uint32(info[8:])&localPath == 0 {
		return "", notRead
	}
	info = info[:size]
	// the path is its base and its suffix, each ended by a zero
	if infoHeader := binary.LittleEndian.Uint32(info[4:]); infoHeader >= 0x24 && size >= 0x24 {
		return wideString(info, binary.LittleEndian.Uint32(info[0x1C:])) + wideString(info, binary.LittleEndian.Uint32(info[0x20:])), nil
	}
	return systemString(append(ansiString(info, binary.LittleEndian.Uint32(info[0x10:])), ansiString(info, binary.LittleEndian.Uint32(info[0x18:]))...))
}

// ansiString is the text at offset of data, up to a zero, as it is stored.
func ansiString(data []byte, offset uint32) []byte {
	if offset == 0 || int(offset) >= len(data) {
		return nil
	}
	text := data[offset:]
	if end := bytes.IndexByte(text, 0); end >= 0 {
		text = text[:end]
	}
	return text
}

// wideString is the text in UTF-16 at offset of data, up to a zero.
func wideString(data []byte, offset uint32) string {
	if offset == 0 || int(offset) >= len(data) {
		return ""
	}
	units := []uint16{}
	for i := int(offset); i+1 < len(data); i += 2 {
		unit := binary.LittleEndian.Uint16(data[i:])
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	return string(utf16.Decode(units))
}

// systemString turns a text in the code page of the system into a string.
func systemString(text []byte) (string, error) {
	if len(text) == 0 {
		return "", nil
	}
	convert := kernel32.NewProc("MultiByteToWideChar")
	const codePageOfTheSystem = 0
	size, _, err := convert.Call(codePageOfTheSystem, 0, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)), 0, 0)
	if size == 0 {
		return "", err
	}
	wide := make([]uint16, size)
	convert.Call(codePageOfTheSystem, 0, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)), uintptr(unsafe.Pointer(&wide[0])), size)
	return string(utf16.Decode(wide)), nil
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

// the COM classes and interfaces of a shortcut
var (
	ole32           = syscall.NewLazyDLL("ole32.dll")
	clsidShellLink  = comGUID{0x00021401, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLinkW   = comGUID{0x000214F9, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIPersistFile = comGUID{0x0000010B, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

type comGUID struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

// the methods used, by their place in the tables of IShellLinkW and IPersistFile
const (
	comQueryInterface   = 0
	comRelease          = 2
	linkSetDescription  = 7
	linkSetIconLocation = 17
	linkSetPath         = 20
	persistSave         = 6
)

// comCall calls the method at index of the table of a COM object, and gives its HRESULT.
func comCall(object unsafe.Pointer, index int, args ...uintptr) uintptr {
	table := *(*unsafe.Pointer)(object)
	method := *(*uintptr)(unsafe.Add(table, uintptr(index)*unsafe.Sizeof(uintptr(0))))
	result, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(object)}, args...)...)
	return result
}

// makeShortcut writes a shortcut at path to target, with the shell of Windows (IShellLinkW).
// Its paths go to the shell as they are, in UTF-16: no script is written with them, so a folder
// named O’Brien, which PowerShell took for the end of a quoted text, works, and a name made to
// run code that way runs nothing.
func makeShortcut(path string, target string) error {
	failed := func(hresult uintptr) error {
		return fmt.Errorf("the shortcut cannot be made: %w", syscall.Errno(hresult))
	}
	pathText, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	targetText, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	description, _ := syscall.UTF16PtrFromString("NXPress")
	// COM works on one thread; one already set for COM by another part of NXPress is used as it is
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const apartmentThreaded = 0x2
	if result, _, _ := ole32.NewProc("CoInitializeEx").Call(0, apartmentThreaded); int32(result) >= 0 {
		defer ole32.NewProc("CoUninitialize").Call()
	}
	var link unsafe.Pointer
	const inProcess = 0x1
	if result, _, _ := ole32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, inProcess,
		uintptr(unsafe.Pointer(&iidShellLinkW)), uintptr(unsafe.Pointer(&link))); int32(result) < 0 {
		return failed(result)
	}
	defer comCall(link, comRelease)
	for _, call := range []struct {
		method int
		args   []uintptr
	}{
		{linkSetPath, []uintptr{uintptr(unsafe.Pointer(targetText))}},
		{linkSetIconLocation, []uintptr{uintptr(unsafe.Pointer(targetText)), 0}},
		{linkSetDescription, []uintptr{uintptr(unsafe.Pointer(description))}},
	} {
		if result := comCall(link, call.method, call.args...); int32(result) < 0 {
			return failed(result)
		}
	}
	var file unsafe.Pointer
	if result := comCall(link, comQueryInterface, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&file))); int32(result) < 0 {
		return failed(result)
	}
	defer comCall(file, comRelease)
	if result := comCall(file, persistSave, uintptr(unsafe.Pointer(pathText)), 1); int32(result) < 0 {
		return failed(result)
	}
	runtime.KeepAlive(pathText)
	runtime.KeepAlive(targetText)
	runtime.KeepAlive(description)
	return nil
}

// archiveBit sets the archive bit of a folder: on a microSD card in FAT32 it makes the
// console read the parts in it as one file.
func archiveBit(path string) error {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return syscall.SetFileAttributes(name, syscall.FILE_ATTRIBUTE_DIRECTORY|syscall.FILE_ATTRIBUTE_ARCHIVE)
}
