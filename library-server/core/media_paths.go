package main

import "strings"

// appDirFor maps a reader surface to its stable top-level directory in the
// media pool. Studio and the reader contract depend on these exact names.
func appDirFor(surface string) (dir string, ok bool) {
	switch surface {
	case "moments":
		return "Moments", true
	case "cabinet":
		return "Cabinet", true
	default:
		return "", false
	}
}

// sanitizeSegment produces one safe collection-directory name: no path
// separators, traversal markers or leading/trailing dots.
func sanitizeSegment(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", " ")
	name = strings.ReplaceAll(name, "\\", " ")
	name = strings.ReplaceAll(name, "..", "")
	name = strings.Trim(name, ". ")
	return strings.Join(strings.Fields(name), " ")
}
