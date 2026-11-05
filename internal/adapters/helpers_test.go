package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

var ctx = context.Background()

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func claudeState(email string) string {
	return `{"numStartups": 7, "theme": "dark", "hasCompletedOnboarding": true, "oauthAccount": {"emailAddress": "` + email + `", "organizationName": "Org"}}`
}

// reads a file from testdata, fixtures are kept as real files so they read like the tools' own configs
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
