package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/vault"
)

func TestBackupEncryptedRestore(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "sub", "b.json") // does not exist yet
	if err := os.WriteFile(a, []byte("SECRET-A"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Dir: filepath.Join(dir, "backups"), Vault: vault.NewMemory()}
	snap, err := Capture([]string{a, b, a})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Files) != 2 {
		t.Fatalf("dedupe failed: %d", len(snap.Files))
	}
	snap.Reason = "switch-to-work"
	path, err := s.Save(snap)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "SECRET-A") {
		t.Fatal("backup is not encrypted")
	}

	// mutate state
	os.WriteFile(a, []byte("CHANGED"), 0o600)
	os.MkdirAll(filepath.Dir(b), 0o700)
	os.WriteFile(b, []byte("NEW"), 0o600)

	got, err := s.Load("latest")
	if err != nil {
		t.Fatal(err)
	}
	if err := RestoreFiles(got); err != nil {
		t.Fatal(err)
	}
	if c, _ := os.ReadFile(a); string(c) != "SECRET-A" {
		t.Fatalf("a = %q", c)
	}
	if _, err := os.Stat(b); !os.IsNotExist(err) {
		t.Fatal("file that did not exist was not removed")
	}
	list, _ := s.List()
	if len(list) != 1 || list[0].Reason != "switch-to-work" {
		t.Fatalf("list = %+v", list)
	}
}

func TestPrune(t *testing.T) {
	s := &Store{Dir: t.TempDir(), Vault: vault.NewMemory()}
	for i := 0; i < Keep+5; i++ {
		snap, _ := Capture(nil)
		snap.Reason = "x"
		if _, err := s.Save(snap); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.List()
	if len(list) != Keep {
		t.Fatalf("kept %d, want %d", len(list), Keep)
	}
}
