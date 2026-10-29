//go:build windows

package main

import (
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileBasicInfo is FILE_BASIC_INFO: the times of a file, with the one its entry changed last.
type fileBasicInfo struct {
	CreationTime   int64
	LastAccessTime int64
	LastWriteTime  int64
	ChangeTime     int64
	FileAttributes uint32
	_              uint32
}

// stampOf reads what tells a file apart through a handle that only reads its attributes: it
// waits for no one and no one waits for it, also while another program copies or reads it.
func stampOf(path string) (fileStamp, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fileStamp{}, err
	}
	// a folder opens only with backup semantics
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return fileStamp{}, &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return fileStamp{}, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	var basic fileBasicInfo
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&basic)), uint32(unsafe.Sizeof(basic))); err != nil {
		return fileStamp{}, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	stamp := fileStamp{
		Size:     int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow),
		Modified: time.Unix(0, info.LastWriteTime.Nanoseconds()),
		Volume:   uint64(info.VolumeSerialNumber),
		Index:    uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow),
		Folder:   info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0,
	}
	// a file system that keeps no time of change gives 0
	if basic.ChangeTime != 0 {
		changed := windows.Filetime{LowDateTime: uint32(basic.ChangeTime), HighDateTime: uint32(basic.ChangeTime >> 32)}
		stamp.Changed = time.Unix(0, changed.Nanoseconds())
	}
	return stamp, nil
}
