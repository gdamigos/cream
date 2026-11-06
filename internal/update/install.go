package update

import (
	"fmt"
	"path"
	"strings"
)

// how cream was installed, so an update hint can show the matching command
type Install struct {
	Method  string // scoop, homebrew, deb, go or manual
	Label   string
	Command string // empty for manual installs
}

// guesses from where the binary lives, nothing is run and nothing is downloaded
func DetectInstall(exe, goos, goarch, latest, home string, getenv func(string) string) Install {
	p := strings.ToLower(strings.ReplaceAll(exe, `\`, "/"))
	slash := func(s string) string {
		return strings.TrimSuffix(strings.ToLower(strings.ReplaceAll(s, `\`, "/")), "/")
	}

	scoop := "/scoop/apps/cream/"
	if s := getenv("SCOOP"); s != "" {
		scoop = slash(s) + "/apps/cream/"
	}
	switch {
	case strings.Contains(p, scoop) || strings.Contains(p, "/scoop/apps/cream/"):
		return Install{Method: "scoop", Label: "Scoop", Command: "scoop update cream"}
	case strings.Contains(p, "/caskroom/cream/") || strings.Contains(p, "/cellar/cream/"):
		return Install{Method: "homebrew", Label: "Homebrew", Command: "brew upgrade --cask cream"}
	case goos == "linux" && p == "/usr/bin/cream":
		v := strings.TrimPrefix(latest, "v")
		if _, ok := parse(v); !ok {
			return Install{Method: "deb", Label: "the .deb package"}
		}
		deb := fmt.Sprintf("cream-go_%s_%s.deb", v, goarch)
		url := fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", Repo, v, deb)
		return Install{Method: "deb", Label: "the .deb package", Command: "curl -fLO " + url + " && sudo apt install ./" + deb}
	}

	bin := slash(getenv("GOBIN"))
	if bin == "" {
		sep := ":"
		if goos == "windows" {
			sep = ";"
		}
		gopath, _, _ := strings.Cut(getenv("GOPATH"), sep)
		if gopath == "" {
			gopath = home + "/go"
		}
		bin = slash(gopath) + "/bin"
	}
	if path.Dir(p) == bin {
		return Install{Method: "go", Label: "go install", Command: "go install github.com/" + Repo + "@latest"}
	}
	return Install{Method: "manual", Label: "a download"}
}

// the release page for a version tag, or the latest release when the tag looks wrong
func ReleaseURL(tag string) string {
	if _, ok := parse(tag); ok && strings.HasPrefix(tag, "v") {
		return "https://github.com/" + Repo + "/releases/tag/" + tag
	}
	return "https://github.com/" + Repo + "/releases/latest"
}
