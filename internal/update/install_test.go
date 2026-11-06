package update

import "testing"

func TestDetectInstall(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	none := env(nil)
	cases := []struct {
		name, exe, goos string
		getenv          func(string) string
		method, command string
	}{
		{"scoop", `C:\Users\me\scoop\apps\cream\current\cream.exe`, "windows", none, "scoop", "scoop update cream"},
		{"scoop elsewhere", `D:\tools\apps\cream\0.1.3\cream.exe`, "windows", env(map[string]string{"SCOOP": `D:\tools`}), "scoop", "scoop update cream"},
		{"homebrew arm", "/opt/homebrew/Caskroom/cream/0.1.3/cream", "darwin", none, "homebrew", "brew upgrade --cask cream"},
		{"homebrew intel", "/usr/local/Caskroom/cream/0.1.3/cream", "darwin", none, "homebrew", "brew upgrade --cask cream"},
		{"deb", "/usr/bin/cream", "linux", none, "deb",
			"curl -fLO https://github.com/gdamigos/cream/releases/download/v0.1.4/cream-go_0.1.4_amd64.deb && sudo apt install ./cream-go_0.1.4_amd64.deb"},
		{"go default", "/home/me/go/bin/cream", "linux", none, "go", "go install github.com/gdamigos/cream@latest"},
		{"go GOBIN", "/opt/gobin/cream", "linux", env(map[string]string{"GOBIN": "/opt/gobin"}), "go", "go install github.com/gdamigos/cream@latest"},
		{"go GOPATH list", `C:\gp\bin\cream.exe`, "windows", env(map[string]string{"GOPATH": `C:\gp;C:\gp2`}), "go", "go install github.com/gdamigos/cream@latest"},
		{"zip download", `C:\Users\me\AppData\Local\Programs\cream\cream.exe`, "windows", none, "manual", ""},
		{"usr/bin on mac is not a deb", "/usr/bin/cream", "darwin", none, "manual", ""},
	}
	for _, c := range cases {
		got := DetectInstall(c.exe, c.goos, "amd64", "v0.1.4", "/home/me", c.getenv)
		if got.Method != c.method || got.Command != c.command {
			t.Errorf("%s: got %+v, want %s %q", c.name, got, c.method, c.command)
		}
	}
}

func TestReleaseURL(t *testing.T) {
	if got := ReleaseURL("v0.1.4"); got != "https://github.com/gdamigos/cream/releases/tag/v0.1.4" {
		t.Fatal(got)
	}
	// a tampered cache must never turn into an arbitrary link
	for _, bad := range []string{"", "v0.1.4/../../evil", "javascript:alert(1)", "0.1.4"} {
		if got := ReleaseURL(bad); got != "https://github.com/gdamigos/cream/releases/latest" {
			t.Errorf("%q gave %s", bad, got)
		}
	}
}
