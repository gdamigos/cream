package fsutil

import "golang.org/x/sys/windows"

// sets the hidden attribute, like windows does for .git
func Hide(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return err
	}
	return windows.SetFileAttributes(p, attrs|windows.FILE_ATTRIBUTE_HIDDEN)
}
