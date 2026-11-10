package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
)

func TestProtectedProfileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	id, _ := age.GenerateX25519Identity()
	c := New()
	p, _ := c.AddProfile("alice", "secret description")
	p.Git = GitIdentity{Name: "Alice", Email: "alice@example.com"}
	p.Rules = []Rule{{Kind: "dir", Pattern: "~/alice"}}
	p.Accounts["claude"] = &Account{Label: "alice@example.com"}
	p.SetProtection(id.Recipient().String())
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	for _, leak := range []string{"secret description", "alice@example.com", "~/alice"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("%q written in plain:\n%s", leak, raw)
		}
	}

	c2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	q := c2.Profiles["alice"]
	if !q.Sealed() || q.Git.Email != "" {
		t.Fatal("inactive protected profile loaded readable")
	}
	other, _ := age.GenerateX25519Identity()
	if q.Open(other) == nil {
		t.Fatal("opened with the wrong key")
	}
	if err := q.Open(id); err != nil || q.Accounts["claude"].Label != "alice@example.com" || q.Description != "secret description" {
		t.Fatalf("open: %v %+v", err, q)
	}

	// the active one is stored readable, closing it seals again without losing anything
	c2.Active = "alice"
	c2.Save(path)
	if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), "alice@example.com") {
		t.Fatal("active protected profile not readable on disk")
	}
	c3, _ := Load(path)
	if c3.Profiles["alice"].Sealed() {
		t.Fatal("active protected profile loaded sealed")
	}
	q3 := c3.Profiles["alice"]
	q3.Close()
	if q3.Open(id); q3.Git.Name != "Alice" || len(q3.Rules) != 1 {
		t.Fatalf("close and open lost data: %+v", q3)
	}
}
