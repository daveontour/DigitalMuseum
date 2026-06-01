//go:build windows

package utils

import (
	"time"

	"golang.org/x/sys/windows"
)

func fileBirthTime(path string) (time.Time, bool) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return time.Time{}, false
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return time.Time{}, false
	}
	defer windows.CloseHandle(handle)

	var created, accessed, written windows.Filetime
	if err := windows.GetFileTime(handle, &created, &accessed, &written); err != nil {
		return time.Time{}, false
	}
	return time.Unix(0, created.Nanoseconds()), true
}
