//go:build !windows && !unix

package main

func studioHasDiskHeadroom(string) (bool, error) {
	return true, nil
}
