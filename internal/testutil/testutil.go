package testutil

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/paths"
	"github.com/gdamigos/cream/internal/vault"
)

type Call struct {
	Name  string
	Args  []string
	Stdin string
}

type FakeRunner struct {
	mu      sync.Mutex
	Calls   []Call
	Outputs map[string]string // keyed by "name arg1 arg2"
	Errors  map[string]error
	Bins    map[string]bool // what LookPath finds
	// these run for real, like git
	Passthrough map[string]bool
}

func NewFakeRunner() *FakeRunner {
	return &FakeRunner{Outputs: map[string]string{}, Errors: map[string]error{}, Bins: map[string]bool{}, Passthrough: map[string]bool{}}
}

func (f *FakeRunner) Run(ctx context.Context, stdin []byte, env []string, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, Call{Name: name, Args: args, Stdin: string(stdin)})
	f.mu.Unlock()
	if f.Passthrough[name] {
		return adapter.ExecRunner{}.Run(ctx, stdin, env, name, args...)
	}
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	if err, ok := f.Errors[key]; ok {
		return nil, err
	}
	if out, ok := f.Outputs[key]; ok {
		return []byte(out), nil
	}
	return nil, errors.New("fake: no output for " + key)
}

func (f *FakeRunner) LookPath(name string) (string, error) {
	if f.Bins[name] || f.Passthrough[name] {
		return "/usr/bin/" + name, nil
	}
	return "", exec.ErrNotFound
}

func (f *FakeRunner) CallStrings() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.Calls {
		out = append(out, strings.TrimSpace(c.Name+" "+strings.Join(c.Args, " ")))
	}
	return out
}

type Env struct {
	*adapter.Env
	Runner *FakeRunner
	Mem    *vault.Memory
	Vars   map[string]string
}

func NewEnv(t *testing.T, goos string) *Env {
	t.Helper()
	home := t.TempDir()
	vars := map[string]string{}
	r := NewFakeRunner()
	mem := vault.NewMemory()
	e := &Env{
		Env: &adapter.Env{
			Paths:  paths.Paths{Home: home, CreamHome: filepath.Join(home, ".config", "cream")},
			Vault:  mem,
			Run:    r,
			GOOS:   goos,
			Config: config.New(),
			Getenv: func(k string) string { return vars[k] },
		},
		Runner: r,
		Mem:    mem,
		Vars:   vars,
	}
	return e
}

func (e *Env) Profile(t *testing.T, name string) *config.Profile {
	t.Helper()
	p, err := e.Config.AddProfile(name, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
