package core

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/vault"
)

type CheckState int

const (
	CheckOK CheckState = iota
	CheckInfo
	CheckWarn
	CheckFail
)

type Check struct {
	Area  string // tools, accounts, git or repo
	Name  string
	State CheckState
	Msg   string
	Fix   string // a command or step that fixes it
}

// adapters whose saved account lives in the vault
var secretAdapters = map[string]bool{"claude": true, "codex": true, "gitlab": true}

// read only: no network, no prompts from cream, nothing is written
func (m *Manager) Doctor(ctx context.Context, dir string) []Check {
	var out []Check
	add := func(c Check) { out = append(out, c) }

	if m.Cfg.Lock.All {
		add(Check{Area: "tools", Name: "lock", State: CheckInfo, Msg: "everything is locked", Fix: "cream unlock"})
	} else if len(m.Cfg.Lock.Profiles) > 0 {
		add(Check{Area: "tools", Name: "lock", State: CheckInfo, Msg: "locked: " + strings.Join(m.Cfg.Lock.Profiles, ", ")})
	}

	active := m.Cfg.Profiles[m.Cfg.Active]
	for _, s := range m.StatusFor(ctx, active) {
		c := Check{Area: "tools", Name: s.Name}
		switch {
		case !s.Live.Installed:
			c.State, c.Msg = CheckInfo, "not installed"
		case s.Live.Warning != "":
			c.State, c.Msg = CheckWarn, s.Live.Warning
		case s.Expected != "" && !s.Match && !m.Cfg.Locked(m.Cfg.Active):
			live := "not logged in"
			if s.Live.LoggedIn {
				live = "logged in as " + s.Live.Account
			}
			c.State, c.Msg = CheckWarn, live+", but "+m.Cfg.Active+" expects "+s.Expected
			c.Fix = "cream use " + m.Cfg.Active + " --only " + s.ID
		case s.Live.LoggedIn:
			c.State, c.Msg = CheckOK, s.Live.Account
		default:
			c.State, c.Msg = CheckInfo, "not logged in"
		}
		add(c)
	}

	for _, name := range m.Cfg.Names() {
		p := m.Cfg.Profiles[name]
		for _, a := range m.Adapters {
			acct := p.Accounts[a.ID()]
			if acct == nil || !secretAdapters[a.ID()] {
				continue
			}
			_, err := m.Env.Vault.Get(vault.ProfileKey(name, a.ID()))
			switch {
			case errors.Is(err, vault.ErrNotFound):
				add(Check{Area: "accounts", Name: name + " · " + a.Name(), State: CheckFail,
					Msg: acct.Label + " is listed but its login is missing from the vault", Fix: "log in as " + acct.Label + ", then cream import " + name + " --only " + a.ID()})
			case err != nil:
				add(Check{Area: "accounts", Name: name + " · " + a.Name(), State: CheckFail, Msg: "vault: " + err.Error()})
			}
		}
	}
	if !slices.ContainsFunc(out, func(c Check) bool { return c.Area == "accounts" }) {
		add(Check{Area: "accounts", Name: "vault", State: CheckOK, Msg: "every saved account is in the " + m.Env.Vault.Backend()})
	}

	out = append(out, m.doctorGit(ctx)...)
	if dir != "" {
		out = append(out, m.doctorRepo(ctx, dir)...)
	}
	return out
}

func (m *Manager) git(ctx context.Context, args ...string) (string, error) {
	b, err := m.Env.Run.Run(ctx, nil, nil, "git", args...)
	return strings.TrimSpace(string(b)), err
}

var gitVersionRe = regexp.MustCompile(`(\d+)\.(\d+)`)

func (m *Manager) doctorGit(ctx context.Context) []Check {
	var out []Check
	if _, err := m.Env.Run.LookPath("git"); err != nil {
		return append(out, Check{Area: "git", Name: "git", State: CheckInfo, Msg: "not installed"})
	}
	v, _ := m.git(ctx, "--version")
	c := Check{Area: "git", Name: "version", State: CheckOK, Msg: strings.TrimPrefix(v, "git version ")}
	if mm := gitVersionRe.FindStringSubmatch(v); mm != nil && hasRemoteRule(m.Cfg) {
		major, _ := strconv.Atoi(mm[1])
		minor, _ := strconv.Atoi(mm[2])
		if major < 2 || (major == 2 && minor < 36) {
			c.State, c.Msg, c.Fix = CheckFail, c.Msg+", remote rules need git 2.36 or newer", "update git"
		}
	}
	out = append(out, c)

	if changes, err := m.SyncGit(true); err != nil {
		out = append(out, Check{Area: "git", Name: "managed block", State: CheckFail, Msg: err.Error()})
	} else if len(changes) > 0 && m.Cfg.Active == "" {
		out = append(out, Check{Area: "git", Name: "managed block", State: CheckInfo, Msg: "not written yet, the first switch does it"})
	} else if len(changes) > 0 {
		out = append(out, Check{Area: "git", Name: "managed block", State: CheckWarn, Msg: "~/.gitconfig doesn't match your profiles and rules",
			Fix: "cream use " + m.Cfg.Active + " --only git"})
	} else {
		out = append(out, Check{Area: "git", Name: "managed block", State: CheckOK, Msg: "up to date"})
	}
	return out
}

func hasRemoteRule(cfg *config.Config) bool {
	for _, p := range cfg.Profiles {
		for _, r := range p.Rules {
			if r.Kind == "remote" {
				return true
			}
		}
	}
	return false
}

func (m *Manager) doctorRepo(ctx context.Context, dir string) []Check {
	top, err := m.git(ctx, "-C", dir, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return nil
	}
	name := "this repo"
	var out []Check
	add := func(state CheckState, msg, fix string) {
		out = append(out, Check{Area: "repo", Name: name, State: state, Msg: msg, Fix: fix})
	}

	var urls []string
	if remotes, err := m.git(ctx, "-C", dir, "config", "--get-regexp", `^remote\..*\.url$`); err == nil {
		for _, line := range strings.Split(remotes, "\n") {
			if _, url, ok := strings.Cut(line, " "); ok {
				urls = append(urls, url)
			}
		}
	}

	// the profiles whose rules match, like git's includeIf would
	var owners []*config.Profile
	for _, pname := range m.Cfg.Names() {
		p := m.Cfg.Profiles[pname]
		for _, r := range p.Rules {
			hit := adapters.DirMatches(r, top, m.Paths.Home, m.Env.OS())
			for _, u := range urls {
				hit = hit || adapters.RemoteMatches(r, u)
			}
			if hit {
				owners = append(owners, p)
				break
			}
		}
	}

	email, _ := m.git(ctx, "-C", dir, "config", "--get", "user.email")
	switch {
	case len(owners) > 1:
		var names []string
		for _, p := range owners {
			names = append(names, p.Name)
		}
		add(CheckWarn, "matches rules of "+strings.Join(names, " and ")+", the last one in ~/.gitconfig wins", "cream rule list")
	case len(owners) == 1 && m.Cfg.Locked(owners[0].Name):
		add(CheckInfo, "belongs to "+owners[0].Name+", which is locked", "cream unlock "+owners[0].Name)
	case len(owners) == 1:
		want := owners[0].Git.Email
		switch {
		case email == "":
			add(CheckFail, "belongs to "+owners[0].Name+" but git has no email here", "cream use "+owners[0].Name+" --only git")
		case !strings.EqualFold(email, want):
			add(CheckFail, "commits as "+email+", but "+owners[0].Name+" owns it and uses "+want, "check `git config --show-origin user.email` in this repo")
		default:
			add(CheckOK, "commits as "+email+" ("+owners[0].Name+")", "")
		}
	case email == "":
		add(CheckWarn, "no rule matches, so git refuses to commit here", fmt.Sprintf("cream rule add <profile> --dir %s", top))
	default:
		add(CheckInfo, "no rule matches, commits as "+email+" (global identity)", "")
	}

	// commits that already went in with another email and aren't pushed yet
	if len(owners) == 1 {
		if log, err := m.git(ctx, "-C", dir, "log", "--format=%ae", "@{upstream}..HEAD"); err == nil && log != "" {
			want, wrong := owners[0].Git.Email, 0
			for _, e := range strings.Split(log, "\n") {
				if !strings.EqualFold(strings.TrimSpace(e), want) {
					wrong++
				}
			}
			if wrong > 0 {
				name = "unpushed commits"
				add(CheckFail, fmt.Sprintf("%d not pushed yet with an email other than %s", wrong, want),
					"fix the author before pushing, e.g. git rebase -r @{upstream} --exec 'git commit --amend --no-edit --reset-author'")
			}
		}
	}
	return out
}
