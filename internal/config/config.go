// config.toml must never hold secrets, only names, labels, rules and settings
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/gdamigos/cream/internal/fsutil"
)

const CurrentVersion = 1

type Config struct {
	Version     int                 `toml:"version"`
	Active      string              `toml:"active"`
	UpdateCheck *bool               `toml:"update_check,omitempty"`
	UsageCheck  bool                `toml:"usage_check,omitempty"` // off unless set, asks anthropic and openai for plan usage
	StaleDays   *int                `toml:"stale_login_days,omitempty"`
	Log         *bool               `toml:"log,omitempty"`
	Adapters    []string            `toml:"adapters"` // tools cream manages, nil means every tool
	UI          UIConfig            `toml:"ui"`
	Vault       VaultConfig         `toml:"vault"`
	Git         GitConfig           `toml:"git"`
	Lock        LockState           `toml:"lock,omitempty"`
	Profiles    map[string]*Profile `toml:"profiles"`
}

// what cream lock turned off, the saved logins themselves live in the vault
type LockState struct {
	All      bool     `toml:"all,omitempty" json:"all"`
	Profiles []string `toml:"profiles,omitempty" json:"profiles"`
}

func (c *Config) Locked(profile string) bool {
	return c.Lock.All || slices.Contains(c.Lock.Profiles, profile)
}

func (c *Config) AnyLocked() bool { return c.Lock.All || len(c.Lock.Profiles) > 0 }

type UIConfig struct {
	Intro        bool     `toml:"intro"`
	DynamicTitle bool     `toml:"dynamic_title"`
	DisabledKeys []string `toml:"disabled_keys"`
	ClickTwice   bool     `toml:"click_twice"` // a clicked key hint runs on the second click
}

type VaultConfig struct {
	Backend string `toml:"backend"`
}

type GitConfig struct {
	// nil means true, false writes the active profile as a global fallback identity
	Strict *bool `toml:"strict,omitempty"`
}

type Profile struct {
	Name        string              `toml:"-"`
	Description string              `toml:"description,omitempty"`
	Color       string              `toml:"color,omitempty"`
	Created     time.Time           `toml:"created"`
	Git         GitIdentity         `toml:"git"`
	Rules       []Rule              `toml:"rules,omitempty"`
	Accounts    map[string]*Account `toml:"accounts,omitempty"`
}

type GitIdentity struct {
	Name       string `toml:"name,omitempty"`
	Email      string `toml:"email,omitempty"`
	SigningKey string `toml:"signing_key,omitempty"`
}

func (g GitIdentity) IsZero() bool { return g.Name == "" && g.Email == "" }

type Rule struct {
	Kind    string `toml:"kind"` // dir or remote
	Pattern string `toml:"pattern"`
}

func (r Rule) String() string { return r.Kind + ":" + r.Pattern }

type Account struct {
	Label    string    `toml:"label"` // email or username, never a token
	Host     string    `toml:"host,omitempty"`
	Imported time.Time `toml:"imported"`
	// when the tool itself last refreshed the login, zero when the tool doesn't record it
	Refreshed time.Time `toml:"refreshed,omitempty"`
}

// the best guess of when the saved login was last refreshed, sync moves Imported on every real refresh
func (a *Account) LastRefresh() time.Time {
	if !a.Refreshed.IsZero() {
		return a.Refreshed
	}
	return a.Imported
}

func New() *Config {
	return &Config{
		Version:  CurrentVersion,
		UI:       UIConfig{Intro: true, DynamicTitle: true, DisabledKeys: []string{}, ClickTwice: true},
		Vault:    VaultConfig{Backend: "auto"},
		Profiles: map[string]*Profile{},
	}
}

func Load(path string) (*Config, error) {
	c := New()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := toml.Decode(string(b), c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Profiles == nil {
		c.Profiles = map[string]*Profile{}
	}
	for name, p := range c.Profiles {
		if p == nil {
			p = &Profile{}
			c.Profiles[name] = p
		}
		p.Name = name
		if p.Accounts == nil {
			p.Accounts = map[string]*Account{}
		}
	}
	if c.Vault.Backend == "" {
		c.Vault.Backend = "auto"
	}
	return c, nil
}

func (c *Config) Save(path string) error {
	var buf bytes.Buffer
	buf.WriteString("# CRE.A.M configuration. No secrets are stored in this file.\n")
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, buf.Bytes(), 0o600)
}

func (c *Config) UpdateCheckEnabled() bool { return c.UpdateCheck == nil || *c.UpdateCheck }

func (c *Config) LogEnabled() bool { return c.Log == nil || *c.Log }

func (c *Config) GitStrict() bool { return c.Git.Strict == nil || *c.Git.Strict }

// days after which a saved login counts as stale, 0 turns the warning off
func (c *Config) StaleLoginDays() int {
	if c.StaleDays == nil {
		return 30
	}
	return max(0, *c.StaleDays)
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: use 1-32 chars of a-z, 0-9, '-' or '_' (starting with a letter or digit)", name)
	}
	return nil
}

func (c *Config) Names() []string {
	out := make([]string, 0, len(c.Profiles))
	for n := range c.Profiles {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (c *Config) Profile(name string) (*Profile, error) {
	p, ok := c.Profiles[name]
	if !ok {
		known := strings.Join(c.Names(), ", ")
		if known == "" {
			known = "none yet, create one with `cream profile add <name>`"
		}
		return nil, fmt.Errorf("unknown profile %q (profiles: %s)", name, known)
	}
	return p, nil
}

var Palette = []string{"#4FB3A9", "#F1E3C3", "#8FB8DE", "#C9A0DC", "#7BD88F", "#E8C468", "#E99B8F"}

func (c *Config) AddProfile(name, description string) (*Profile, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	if _, ok := c.Profiles[name]; ok {
		return nil, fmt.Errorf("profile %q already exists", name)
	}
	p := &Profile{
		Name:        name,
		Description: description,
		Color:       Palette[len(c.Profiles)%len(Palette)],
		Created:     time.Now().UTC().Truncate(time.Second),
		Accounts:    map[string]*Account{},
	}
	c.Profiles[name] = p
	return p, nil
}

func (c *Config) RemoveProfile(name string) error {
	if _, err := c.Profile(name); err != nil {
		return err
	}
	delete(c.Profiles, name)
	if c.Active == name {
		c.Active = ""
	}
	c.Lock.Profiles = slices.DeleteFunc(c.Lock.Profiles, func(p string) bool { return p == name })
	return nil
}

func ValidateRule(r Rule) (Rule, error) {
	r.Pattern = strings.TrimSpace(r.Pattern)
	if r.Pattern == "" {
		return r, errors.New("rule pattern is empty")
	}
	if strings.ContainsAny(r.Pattern, "\"\n\r") {
		return r, errors.New("rule pattern may not contain quotes or newlines")
	}
	switch r.Kind {
	case "dir":
	case "remote":
		r.Pattern = strings.TrimPrefix(strings.TrimPrefix(r.Pattern, "https://"), "http://")
		r.Pattern = strings.TrimSuffix(r.Pattern, "/")
	default:
		return r, fmt.Errorf("unknown rule kind %q (want dir or remote)", r.Kind)
	}
	if strings.TrimSpace(r.Pattern) == "" {
		return r, errors.New("rule pattern is empty")
	}
	return r, nil
}
