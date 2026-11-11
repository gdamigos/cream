package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringChunking(t *testing.T) {
	keyring.MockInit()
	k := Keyring{}
	small := []byte("hello")
	big := bytes.Repeat([]byte("0123456789abcdef"), 600) // big enough to force chunks
	for _, v := range [][]byte{small, big} {
		if err := k.Set("profile:w:codex", v); err != nil {
			t.Fatal(err)
		}
		got, err := k.Get("profile:w:codex")
		if err != nil || !bytes.Equal(got, v) {
			t.Fatalf("roundtrip failed for %d bytes: %v", len(v), err)
		}
	}
	for i := 0; ; i++ {
		part, err := keyring.Get(Service, chunkKey("profile:w:codex", i))
		if err != nil {
			if i == 0 {
				t.Fatal("expected chunks")
			}
			break
		}
		if len(part) > 2560 {
			t.Fatalf("chunk %d too large: %d", i, len(part))
		}
	}
	if err := k.Delete("profile:w:codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get("profile:w:codex"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := keyring.Get(Service, chunkKey("profile:w:codex", 0)); err == nil {
		t.Fatal("chunks left behind after delete")
	}
}

func TestFileVaultEncryptedRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.age")
	v, err := NewFile(path, "correct horse", 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set("k", []byte("TOP-SECRET")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "TOP-SECRET") {
		t.Fatal("plaintext secret on disk")
	}
	if !strings.HasPrefix(string(raw), "age-encryption.org/v1") {
		t.Fatal("not an age file")
	}
	v2, _ := NewFile(path, "correct horse", 10)
	if got, err := v2.Get("k"); err != nil || string(got) != "TOP-SECRET" {
		t.Fatalf("%q %v", got, err)
	}
	v3, _ := NewFile(path, "wrong", 10)
	if _, err := v3.Get("k"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	if _, err := NewFile(path, "", 10); err == nil {
		t.Fatal("empty passphrase accepted")
	}
}

func TestFileVaultKeepsOtherProcessWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.age")
	a, _ := NewFile(path, "correct horse", 10)
	b, _ := NewFile(path, "correct horse", 10)
	if err := a.Set("old", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Get("old"); err != nil {
		t.Fatal(err)
	}
	// b loaded the file before a wrote again, like two cream processes
	if err := a.Set("from-a", []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := b.Set("from-b", []byte("3")); err != nil {
		t.Fatal(err)
	}
	c, _ := NewFile(path, "correct horse", 10)
	for _, k := range []string{"old", "from-a", "from-b"} {
		if _, err := c.Get(k); err != nil {
			t.Fatalf("%s lost: %v", k, err)
		}
	}
	if got, err := a.Get("from-b"); err != nil || string(got) != "3" {
		t.Fatalf("a doesn't see b's write: %q %v", got, err)
	}
}

func TestOpenFileBackendFromEnv(t *testing.T) {
	t.Setenv("CREAM_VAULT_PASSPHRASE", "pw")
	v, err := Open(OpenOptions{Backend: "file", FilePath: filepath.Join(t.TempDir(), "v.age")})
	if err != nil || v.Backend() != "file" {
		t.Fatalf("%v %v", v, err)
	}
	if _, err := Open(OpenOptions{Backend: "nope"}); err == nil {
		t.Fatal("bad backend accepted")
	}
}
