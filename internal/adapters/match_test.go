package adapters

import (
	"testing"

	"github.com/gdamigos/cream/internal/config"
)

func TestRemoteMatches(t *testing.T) {
	host := config.Rule{Kind: "remote", Pattern: "gitlab.corp.com"}
	group := config.Rule{Kind: "remote", Pattern: "github.com/acme"}
	cases := []struct {
		rule config.Rule
		url  string
		want bool
	}{
		{host, "https://gitlab.corp.com/team/app.git", true},
		{host, "https://me@gitlab.corp.com/team/app", true},
		{host, "ssh://git@gitlab.corp.com:2222/team/app.git", true},
		{host, "git@gitlab.corp.com:team/app.git", true},
		{host, "git@GitLab.Corp.com:team/app.git", true},
		{host, "https://gitlab.corp.com.evil.io/team/app.git", false},
		{host, "https://github.com/team/app.git", false},
		{group, "git@github.com:acme/app.git", true},
		{group, "https://github.com/acme-other/app.git", false},
		{config.Rule{Kind: "dir", Pattern: "gitlab.corp.com"}, "https://gitlab.corp.com/x", false},
	}
	for _, c := range cases {
		if got := RemoteMatches(c.rule, c.url); got != c.want {
			t.Errorf("%s vs %s: got %v", c.rule, c.url, got)
		}
	}
}

func TestDirMatches(t *testing.T) {
	r := config.Rule{Kind: "dir", Pattern: "~/work"}
	if !DirMatches(r, "/home/me/work/app", "/home/me", "linux") || !DirMatches(r, "/home/me/work", "/home/me", "linux") {
		t.Fatal("repo under the folder should match")
	}
	if DirMatches(r, "/home/me/workshop/app", "/home/me", "linux") {
		t.Fatal("a sibling folder with the same prefix must not match")
	}
	if !DirMatches(config.Rule{Kind: "dir", Pattern: `C:\Code\Work`}, `c:/code/work/app`, `C:\Users\me`, "windows") {
		t.Fatal("windows paths are case insensitive")
	}
	if DirMatches(config.Rule{Kind: "dir", Pattern: "/Code/Work"}, "/code/work/app", "/home/me", "linux") {
		t.Fatal("linux paths are case sensitive")
	}
}
