//go:build unix

package main

import "syscall"

func studioHasDiskHeadroom(path string) (bool, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false, err
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	total := uint64(stat.Blocks) * uint64(stat.Bsize)
	minimum := uint64(1 << 30)
	if tenPercent := total / 10; tenPercent > minimum {
		minimum = tenPercent
	}
	return available > minimum, nil
}
