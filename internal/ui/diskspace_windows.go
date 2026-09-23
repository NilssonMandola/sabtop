//go:build windows

package ui

import "golang.org/x/sys/windows"

// volumeUsage reports free and total bytes for the volume holding path.
// See the Unix implementation for why sabtop measures these paths itself.
func volumeUsage(path string) (free, total uint64, ok bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	var avail, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &totalBytes, &totalFree); err != nil {
		return 0, 0, false
	}
	if totalBytes == 0 {
		return 0, 0, false
	}
	// avail is what this user may actually use, the analogue of Bavail.
	return avail, totalBytes, true
}
