//go:build linux

package library

import (
	"fmt"
	"syscall"
)

var filesystemNames = map[int64]string{
	0xEF53:     "ext4",
	0x58465342: "xfs",
	0x9123683E: "btrfs",
	0x794C7630: "overlay",
	0x01021994: "tmpfs",
	0x6969:     "nfs",
	0xFF534D42: "cifs",
	0xFE534D42: "smb2",
	0x4D44:     "vfat",
	0x2011BAB0: "exfat",
	0x65735546: "fuse",
	0x2FC12FC1: "zfs",
	0x858458F6: "ramfs",
	0x73717368: "squashfs",
	0x01021997: "9p",
	0x62656572: "sysfs",
	0x9FA0:     "proc",
	0x1CD1:     "devpts",
	0x42494E4D: "binfmt_misc",
}

// volumeDetailsFor 在 Linux 只回報容器／主機看到的檔案系統類型；GUID 由 Windows 原生模式提供。
func volumeDetailsFor(path string) (volumeID, relative, filesystem string) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "", "", ""
	}
	return "", "", filesystemName(int64(stat.Type))
}

func filesystemName(kind int64) string {
	if name, ok := filesystemNames[kind]; ok {
		return name
	}
	return fmt.Sprintf("0x%x", uint64(kind))
}

func resolveVolumePath(volumeID, relative string) (string, error) {
	return "", fmt.Errorf("此平台不支援磁碟區 GUID")
}

func volumeTrackingSupported() bool { return false }
