package main

import (
	"context"
	_ "embed"
	"os"
	"strings"

	"github.com/gdamigos/cream/internal/cli"
	"github.com/gdamigos/cream/internal/core"
	"github.com/gdamigos/cream/internal/tui"
)

// set by goreleaser ldflags
var (
	version = ""
	commit  = "none"
	date    = "unknown"
)

// bumping VERSION on main triggers a release
//
//go:embed VERSION
var versionFile string

func resolvedVersion() string {
	if version != "" {
		return version
	}
	return "v" + strings.TrimSpace(versionFile) + "-dev"
}

func main() {
	v := resolvedVersion()
	app := &cli.App{
		Build: cli.BuildInfo{Version: v, Commit: commit, Date: date},
		RunTUI: func(a *cli.App, m *core.Manager) error {
			return tui.Run(m, tui.Options{
				Version:     v,
				DryRun:      a.DryRun,
				UpdateCheck: a.UpdateCheckAllowed(m),
				Checker:     a.UpdateChecker(m),
				SkipIntro:   os.Getenv("CREAM_NO_INTRO") != "" || !m.Cfg.UI.Intro,
			})
		},
	}
	os.Exit(cli.Execute(context.Background(), app, os.Args[1:]))
}
