package fsutil

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestHideSetsHiddenAndSurvivesRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".cream")
	if err := WriteFileAtomic(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Hide(path); err != nil {
		t.Fatal(err)
	}
	p, _ := windows.UTF16PtrFromString(path)
	if attrs, _ := windows.GetFileAttributes(p); attrs&windows.FILE_ATTRIBUTE_HIDDEN == 0 {
		t.Fatal("not hidden")
	}
	// rewriting a hidden file must still work
	if err := WriteFileAtomic(path, []byte("b"), 0o644); err != nil {
		t.Fatalf("rewrite hidden file: %v", err)
	}
}
