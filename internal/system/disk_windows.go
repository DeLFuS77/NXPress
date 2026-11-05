package system

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// IOCTL_STORAGE_QUERY_PROPERTY, and the property that tells the seek penalty of a drive
	ioctlStorageQueryProperty        = 0x2D1400
	storageDeviceSeekPenaltyProperty = 7
)

// SeekPenalty tells whether the drive of a path loses time going from one place to another, as a
// hard disk does and an SSD does not, as Windows tells it; known is false when it cannot tell (a
// network folder, a drive that does not say).
func SeekPenalty(path string) (penalty bool, known bool) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false, false
	}
	from, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return false, false
	}
	root := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(from, &root[0], uint32(len(root))); err != nil {
		return false, false
	}
	volume := make([]uint16, 64)
	if err := windows.GetVolumeNameForVolumeMountPoint(&root[0], &volume[0], uint32(len(volume))); err != nil {
		return false, false
	}
	// \\?\Volume{...}\ opened without its last backslash is the volume itself; no access is
	// asked of it, so no administrator is needed
	device, err := windows.UTF16PtrFromString(strings.TrimSuffix(windows.UTF16ToString(volume), `\`))
	if err != nil {
		return false, false
	}
	handle, err := windows.CreateFile(device, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return false, false
	}
	defer windows.CloseHandle(handle)
	query := struct {
		propertyID uint32
		queryType  uint32
		additional [4]byte
	}{propertyID: storageDeviceSeekPenaltyProperty}
	var descriptor struct {
		version           uint32
		size              uint32
		incursSeekPenalty byte
		_                 [3]byte
	}
	var returned uint32
	err = windows.DeviceIoControl(handle, ioctlStorageQueryProperty, (*byte)(unsafe.Pointer(&query)), uint32(unsafe.Sizeof(query)),
		(*byte)(unsafe.Pointer(&descriptor)), uint32(unsafe.Sizeof(descriptor)), &returned, nil)
	if err != nil || returned < 9 {
		return false, false
	}
	return descriptor.incursSeekPenalty != 0, true
}
