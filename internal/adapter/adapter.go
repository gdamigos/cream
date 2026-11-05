package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/paths"
	"github.com/gdamigos/cream/internal/vault"
)

// tests replace every field, so adapters must use Env() and OS() instead of os/runtime
type Env struct {
	Paths  paths.Paths
	Vault  vault.Vault
	Run    Runner
	GOOS   string
	Config *config.Config
	Getenv func(string) string
}

func (e *Env) Env(key string) string {
	if e.Getenv != nil {
		return e.Getenv(key)
	}
	return os.Getenv(key)
}

func (e *Env) OS() string {
	if e.GOOS != "" {
		return e.GOOS
	}
	return runtime.GOOS
}

// every field must be secret free
type Status struct {
	Installed bool
	LoggedIn  bool
	Account   string
	Detail    string
	Warning   string
}

type Change struct {
	Adapter string
	Kind    string // file | keyring | exec | gitconfig
	Target  string
	Action  string
}

func (c Change) String() string {
	return fmt.Sprintf("%-8s %-9s %-7s %s", c.Adapter, c.Kind, c.Action, c.Target)
}

// non-file state kept in backups, like a macos keychain item
type Snapshot map[string][]byte

type Adapter interface {
	ID() string
	Name() string
	// read only, no prompts, no network
	Status(ctx context.Context, env *Env) Status
	// secrets go to the vault, labels to the profile
	Import(ctx context.Context, env *Env, p *config.Profile) (*config.Account, error)
	Activate(ctx context.Context, env *Env, p *config.Profile, dryRun bool) ([]Change, error)
	// every file activate may touch, so it gets backed up
	Files(env *Env) []string
	// finish runs after the child exits to sync refreshed tokens back
	SessionEnv(ctx context.Context, env *Env, p *config.Profile) (vars []string, finish func() error, err error)
	// may hit the network through the tool itself, only on explicit request
	Verify(ctx context.Context, env *Env) (string, error)
}

type Snapshotter interface {
	Snapshot(env *Env) (Snapshot, error)
	RestoreSnapshot(env *Env, s Snapshot) error
}

type ErrNotImported struct{ Adapter, Profile string }

func (e ErrNotImported) Error() string {
	return fmt.Sprintf("%s: profile %q has no saved account (run `cream import %s --only %s`)", e.Adapter, e.Profile, e.Profile, e.Adapter)
}

// skips the adapter without failing the switch
type SkipError struct{ Reason string }

func (e SkipError) Error() string { return e.Reason }

// secrets go through stdin or env, never args
type Runner interface {
	Run(ctx context.Context, stdin []byte, extraEnv []string, name string, args ...string) (stdout []byte, err error)
	LookPath(name string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, stdin []byte, extraEnv []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Env = append(os.Environ(), extraEnv...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return out.Bytes(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return out.Bytes(), nil
}

func (ExecRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }
