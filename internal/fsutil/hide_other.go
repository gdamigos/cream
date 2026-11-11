//go:build !windows

package fsutil

// a leading dot already hides the file here
func Hide(string) error { return nil }
