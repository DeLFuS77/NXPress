package trash

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// The Recycle Bin of Windows, through SHFileOperationW of the shell: the file goes there as the
// Explorer sends it, and Restore puts it back where it was. Before it is asked, NXPress tells the
// drives the shell would delete from for good without a word: a network folder, a removable drive,
// a drive whose Recycle Bin is turned off, a file bigger than its Recycle Bin holds and a path
// longer than the shell takes (with no confirmation asked, the shell deletes such a file for good).

var (
	shell32                = windows.NewLazySystemDLL("shell32.dll")
	procSHFileOperationW   = shell32.NewProc("SHFileOperationW")
	procSHQueryRecycleBinW = shell32.NewProc("SHQueryRecycleBinW")
)

const (
	foDelete = 0x0003
	// no progress, no question, no error shown, and into the Recycle Bin; a file the Recycle Bin
	// cannot take is asked about rather than deleted for good without a word
	fofSilent          = 0x0004
	fofNoConfirmation  = 0x0010
	fofAllowUndo       = 0x0040
	fofNoErrorUI       = 0x0400
	fofWantNukeWarning = 0x4000
)

// shFileOpStruct is SHFILEOPSTRUCTW as 64-bit Windows lays it out (32-bit Windows packs it to one
// byte, and NXPress is not built for it).
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

// shQueryRBInfo is SHQUERYRBINFO.
type shQueryRBInfo struct {
	cbSize      uint32
	i64Size     int64
	i64NumItems int64
}

// maxPath is the longest path the shell takes, in characters (MAX_PATH less its ending NUL).
const maxPath = 259

func move(path string) error {
	if err := check(path); err != nil {
		return err
	}
	// the list of paths the shell takes: each ends in a NUL, and the list in another one
	list := append(wide(path), 0)
	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &list[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI | fofWantNukeWarning,
	}
	// the shell works in a thread of its own with COM, as the Explorer does
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
		defer windows.CoUninitialize()
	}
	result, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	switch {
	case op.fAnyOperationsAborted != 0:
		// the shell would have deleted it for good, and the answer was no
		return &Unavailable{Reason: TooBig, Drive: driveOf(path)}
	case result != 0:
		return fmt.Errorf("the recycle bin did not take it (error 0x%X)", result)
	}
	if _, err := os.Lstat(path); err == nil {
		return errors.New("the recycle bin did not take it")
	}
	return nil
}

// wide is s in UTF-16 with its ending NUL, as Windows takes it.
func wide(s string) []uint16 {
	w, err := windows.UTF16FromString(s)
	if err != nil {
		return []uint16{0}
	}
	return w
}

// length is the length of s in the characters of Windows.
func length(s string) int {
	return len(wide(s)) - 1
}

func check(path string) error {
	drive := driveOf(path)
	if drive == "" {
		return &Unavailable{Reason: NoBin, Drive: volumeName(path)}
	}
	bin, err := binOf(drive)
	if err != nil || bin.none {
		return &Unavailable{Reason: NoBin, Drive: drive}
	}
	// a path longer than the shell takes, there or in the Recycle Bin, is deleted for good
	// without a word, even by its short name (tried on Windows 11)
	if tooLong(path, drive) {
		return &Unavailable{Reason: TooLong, Drive: drive}
	}
	if sizeOf(path) > bin.capacity {
		return &Unavailable{Reason: TooBig, Drive: drive}
	}
	return nil
}

// tooLong tells whether path, or a path in it for a folder, is longer than the shell takes, where
// it is or in the Recycle Bin of its drive, under a name such as $Recycle.Bin\<user>\$RAB12CD.
func tooLong(path string, drive string) bool {
	if length(path) > maxPath {
		return true
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	base := max(length(path), length(drive)+length(`$Recycle.Bin\`)+length(userSid())+length(`\$RAB12CD`))
	long := false
	filepath.WalkDir(path, func(inside string, entry fs.DirEntry, err error) error {
		if base+length(inside)-length(path) > maxPath {
			long = true
			return filepath.SkipAll
		}
		return nil
	})
	return long
}

// userSid is the security identifier of the user, which names the folder of their Recycle Bin.
var userSid = sync.OnceValue(func() string {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return strings.Repeat("S", 48)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return strings.Repeat("S", 48)
	}
	return user.User.Sid.String()
})

// isNetwork tells a path on a network share, \\server\share (\\?\UNC\server\share too), which has
// no Recycle Bin; one through a drive letter is told by the type of its drive.
func isNetwork(path string) bool {
	if strings.HasPrefix(path, `\\?\UNC\`) {
		return true
	}
	return strings.HasPrefix(path, `\\`) && !strings.HasPrefix(path, `\\?\`) && !strings.HasPrefix(path, `\\.\`)
}

// volumeName is the share of a network path, \\server\share, to tell it.
func volumeName(path string) string {
	parts := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(path, `\\?\UNC\`), `\\`), `\`, 3)
	if len(parts) >= 2 {
		return `\\` + parts[0] + `\` + parts[1]
	}
	return path
}

// driveOf is the root of the drive of path, as C:\ or the folder a drive is mounted on; "" for a
// network share.
func driveOf(path string) string {
	if isNetwork(path) {
		return ""
	}
	from := path
	if len(wide(path)) > maxPath && !strings.HasPrefix(path, `\\?\`) {
		from = `\\?\` + path
	}
	pointer, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return ""
	}
	root := make([]uint16, 1024)
	if err := windows.GetVolumePathName(pointer, &root[0], uint32(len(root))); err != nil {
		return ""
	}
	return strings.TrimPrefix(windows.UTF16ToString(root), `\\?\`)
}

// bin is what NXPress knows of the Recycle Bin of a drive: none, or how much it holds.
type bin struct {
	none     bool
	capacity int64
}

// bins keeps what was found of each drive for a minute: a work checks every file it is given.
var bins = struct {
	sync.Mutex
	found map[string]bin
	at    map[string]time.Time
}{found: map[string]bin{}, at: map[string]time.Time{}}

func binOf(drive string) (bin, error) {
	bins.Lock()
	defer bins.Unlock()
	if found, ok := bins.found[drive]; ok && time.Since(bins.at[drive]) < time.Minute {
		return found, nil
	}
	found, err := readBin(drive)
	if err != nil {
		return bin{}, err
	}
	bins.found[drive], bins.at[drive] = found, time.Now()
	return found, nil
}

func readBin(drive string) (bin, error) {
	root, err := windows.UTF16PtrFromString(drive)
	if err != nil {
		return bin{}, err
	}
	// only a fixed drive has a Recycle Bin: a network drive, a removable one (a USB stick, a
	// memory card), a CD and a RAM disk delete for good
	if windows.GetDriveType(root) != windows.DRIVE_FIXED {
		return bin{none: true}, nil
	}
	// the policy that turns the Recycle Bin off on every drive
	if policyNoRecycle() {
		return bin{none: true}, nil
	}
	info := shQueryRBInfo{cbSize: uint32(unsafe.Sizeof(shQueryRBInfo{}))}
	if result, _, _ := procSHQueryRecycleBinW.Call(uintptr(unsafe.Pointer(root)), uintptr(unsafe.Pointer(&info))); result != 0 {
		return bin{none: true}, nil
	}
	// its settings, by the volume: turned off ("Don't move files to the Recycle Bin"), and its
	// maximum size, in MB
	nuke, capacity, found := volumeSettings(drive)
	if nuke {
		return bin{none: true}, nil
	}
	if !found {
		capacity = defaultCapacity(drive)
	}
	return bin{capacity: capacity}, nil
}

// policyNoRecycle tells whether a policy turns the Recycle Bin off: files are deleted for good.
func policyNoRecycle() bool {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		key, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\Policies\Explorer`, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		value, _, err := key.GetIntegerValue("NoRecycleFiles")
		key.Close()
		if err == nil && value == 1 {
			return true
		}
	}
	return false
}

// volumeSettings reads the settings of the Recycle Bin of a drive, kept by the volume: whether it
// deletes for good, and its maximum size in bytes; found is false when it has none written.
func volumeSettings(drive string) (nuke bool, capacity int64, found bool) {
	root, err := windows.UTF16PtrFromString(drive)
	if err != nil {
		return false, 0, false
	}
	name := make([]uint16, 64)
	if err := windows.GetVolumeNameForVolumeMountPoint(root, &name[0], uint32(len(name))); err != nil {
		return false, 0, false
	}
	// \\?\Volume{GUID}\ is kept by its {GUID}
	volume := strings.TrimSuffix(strings.TrimPrefix(windows.UTF16ToString(name), `\\?\Volume`), `\`)
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\BitBucket\Volume\`+volume, registry.QUERY_VALUE)
	if err != nil {
		return false, 0, false
	}
	defer key.Close()
	if value, _, err := key.GetIntegerValue("NukeOnDelete"); err == nil && value == 1 {
		return true, 0, true
	}
	megabytes, _, err := key.GetIntegerValue("MaxCapacity")
	if err != nil {
		return false, 0, false
	}
	return false, int64(megabytes) << 20, true
}

// defaultCapacity is the size Windows gives the Recycle Bin of a drive it has no settings for:
// 10 % of the first 40 GB of the drive, and 5 % of the rest.
func defaultCapacity(drive string) int64 {
	root, err := windows.UTF16PtrFromString(drive)
	if err != nil {
		return 0
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(root, &available, &total, &free); err != nil {
		return 0
	}
	return capacityFor(int64(total>>20)) << 20
}

// capacityFor is that size in MB, for a drive of so many MB.
func capacityFor(megabytes int64) int64 {
	first := min(megabytes, 40960)
	return first/10 + max(0, megabytes-40960)/20
}
