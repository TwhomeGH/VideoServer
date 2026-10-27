//go:build windows

package library

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32                             = syscall.NewLazyDLL("kernel32.dll")
	procGetVolumePathName                = kernel32.NewProc("GetVolumePathNameW")
	procGetVolumeNameForVolumeMountPoint = kernel32.NewProc("GetVolumeNameForVolumeMountPointW")
	procGetVolumeInformation             = kernel32.NewProc("GetVolumeInformationW")
)

// volumeDetailsFor 回傳 Windows volume GUID、相對於掛載點的路徑與檔案系統名稱。
// 網路路徑或無法查詢磁碟區時，volume ID 可能是空字串，仍會保留檔案系統（若可取得）。
func volumeDetailsFor(path string) (volumeID, relative, filesystem string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", ""
	}
	mount, err := volumePathName(abs)
	if err != nil {
		return "", "", ""
	}
	filesystem = volumeFileSystem(mount)
	volumeID, err = volumeName(mount)
	if err != nil {
		return "", "", filesystem
	}
	relative, err = filepath.Rel(mount, abs)
	if err != nil {
		return "", "", filesystem
	}
	if relative == "." {
		relative = ""
	}
	return volumeID, relative, filesystem
}

func volumePathName(path string) (string, error) {
	input, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 32768)
	result, _, callErr := procGetVolumePathName.Call(
		uintptr(unsafe.Pointer(input)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if result == 0 {
		return "", fmt.Errorf("GetVolumePathNameW: %w", callErr)
	}
	return syscall.UTF16ToString(buffer), nil
}

func volumeName(mount string) (string, error) {
	if !strings.HasSuffix(mount, `\`) {
		mount += `\`
	}
	input, err := syscall.UTF16PtrFromString(mount)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 1024)
	result, _, callErr := procGetVolumeNameForVolumeMountPoint.Call(
		uintptr(unsafe.Pointer(input)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if result == 0 {
		return "", fmt.Errorf("GetVolumeNameForVolumeMountPointW: %w", callErr)
	}
	return syscall.UTF16ToString(buffer), nil
}

func volumeFileSystem(mount string) string {
	if !strings.HasSuffix(mount, `\`) {
		mount += `\`
	}
	input, err := syscall.UTF16PtrFromString(mount)
	if err != nil {
		return ""
	}
	volumeNameBuffer := make([]uint16, 261)
	fileSystemBuffer := make([]uint16, 261)
	var serial, maxComponent, flags uint32
	result, _, _ := procGetVolumeInformation.Call(
		uintptr(unsafe.Pointer(input)),
		uintptr(unsafe.Pointer(&volumeNameBuffer[0])), uintptr(len(volumeNameBuffer)),
		uintptr(unsafe.Pointer(&serial)),
		uintptr(unsafe.Pointer(&maxComponent)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&fileSystemBuffer[0])), uintptr(len(fileSystemBuffer)),
	)
	if result == 0 {
		return ""
	}
	return syscall.UTF16ToString(fileSystemBuffer)
}

func resolveVolumePath(volumeID, relative string) (string, error) {
	if volumeID == "" {
		return "", fmt.Errorf("缺少磁碟區 GUID")
	}
	if relative == "" {
		return volumeID, nil
	}
	return filepath.Join(volumeID, relative), nil
}
