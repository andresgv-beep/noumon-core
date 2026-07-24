//go:build unix

package main

import "os"

func replaceStudioFile(source, destination string) error {
	return os.Rename(source, destination)
}
