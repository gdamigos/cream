package config

import (
	"strings"
	"testing"
)

// rule patterns end up inside ~/.gitconfig, so an accepted one must never be able to break out of its quotes
func FuzzValidateRule(f *testing.F) {
	for _, s := range []string{"gitlab.company.com", "https://github.com/", "~/work", `C:\work`, "a\"b", "x\ny", ""} {
		f.Add("remote", s)
		f.Add("dir", s)
	}
	f.Fuzz(func(t *testing.T, kind, pattern string) {
		r, err := ValidateRule(Rule{Kind: kind, Pattern: pattern})
		if err != nil {
			return
		}
		if r.Pattern == "" || strings.ContainsAny(r.Pattern, "\"\n\r") {
			t.Fatalf("accepted unsafe pattern %q", r.Pattern)
		}
		if kind != "dir" && kind != "remote" {
			t.Fatalf("accepted unknown kind %q", kind)
		}
	})
}

// profile names become vault keys and file names
func FuzzValidateName(f *testing.F) {
	for _, s := range []string{"work", "personal", "a", "../x", "Work", "", strings.Repeat("a", 40)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if ValidateName(name) != nil {
			return
		}
		if len(name) > 32 || strings.ContainsAny(name, `/\:. `) || strings.ToLower(name) != name {
			t.Fatalf("accepted unsafe name %q", name)
		}
	})
}
