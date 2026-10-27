//go:build !windows && !linux

package library

import "fmt"

// volumeDetailsFor 在未支援磁碟區身分的平台只保留路徑模式。
func volumeDetailsFor(path string) (volumeID, relative, filesystem string) {
	return "", "", ""
}

func resolveVolumePath(volumeID, relative string) (string, error) {
	return "", fmt.Errorf("此平台不支援磁碟區 GUID")
}

func volumeTrackingSupported() bool { return false }
