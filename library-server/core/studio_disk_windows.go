//go:build windows

package main

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func studioHasDiskHeadroom(path string) (bool, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	ptr, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return false, err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &available, &total, &free); err != nil {
		return false, err
	}
	minimum := uint64(1 << 30)
	if tenPercent := total / 10; tenPercent > minimum {
		minimum = tenPercent
	}
	return available > minimum, nil
}
