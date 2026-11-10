package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/vault"
)

// the caller asks for the profile's password, calls Unseal and runs the operation again
type PasswordRequired struct{ Profile string }

func (e PasswordRequired) Error() string {
	return fmt.Sprintf("profile %q has a password, enter it to continue", e.Profile)
}

var ErrWrongPassword = errors.New("wrong password")

const MinPasswordLen = 8

func (m *Manager) Protected(name string) bool {
	p := m.Cfg.Profiles[name]
	return p != nil && p.Protected()
}

// protected and its password wasn't entered in this process yet
func (m *Manager) NeedsPassword(name string) bool { return m.Protected(name) && m.keys[name] == nil }

// the vault under the encrypting wrapper
func (m *Manager) raw() vault.Vault {
	if sv, ok := m.Env.Vault.(*sealVault); ok {
		return sv.Vault
	}
	return m.Env.Vault
}

// checks the password and keeps the profile's key in memory until Reseal
func (m *Manager) Unseal(name, password string) error {
	p, err := m.Cfg.Profile(name)
	if err != nil {
		return err
	}
	if !p.Protected() {
		return nil
	}
	b, err := m.raw().Get(vault.ProtectKey(name))
	if errors.Is(err, vault.ErrNotFound) {
		return fmt.Errorf("profile %q has lost its key, its saved data can't be opened (delete it with `cream profile remove %s --forgot-password`)", name, name)
	}
	if err != nil {
		return err
	}
	sid, err := age.NewScryptIdentity(password)
	if err != nil {
		return err
	}
	r, err := age.Decrypt(bytes.NewReader(b), sid)
	if err != nil {
		return fmt.Errorf("%w for %s", ErrWrongPassword, name)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	id, err := age.ParseX25519Identity(strings.TrimSpace(string(plain)))
	if err != nil {
		return fmt.Errorf("profile %q: corrupt key: %w", name, err)
	}
	if err := p.Open(id); err != nil {
		return err
	}
	m.keys[name] = id
	return nil
}

// an empty password reuses the key entered earlier in this process
func (m *Manager) unsealWith(name, password string) error {
	if password == "" && m.keys[name] != nil {
		return nil
	}
	return m.Unseal(name, password)
}

// forgets every entered password and encrypts the profiles they opened again, the active one stays readable
func (m *Manager) Reseal() {
	clear(m.keys)
	for name, p := range m.Cfg.Profiles {
		if name != m.Cfg.Active {
			_ = p.Close()
		}
	}
}

// the profile with its fields readable, PasswordRequired when it is protected and locked
func (m *Manager) Profile(name string) (*config.Profile, error) {
	p, err := m.Cfg.Profile(name)
	if err != nil {
		return nil, err
	}
	if !p.Protected() {
		return p, nil
	}
	id := m.keys[name]
	if id == nil {
		return nil, PasswordRequired{name}
	}
	return p, p.Open(id)
}

// the active profile when it is protected, its live logins make backups and lock entries private
func (m *Manager) protectedActive() string {
	if m.Protected(m.Cfg.Active) {
		return m.Cfg.Active
	}
	return ""
}

func (m *Manager) storeKey(name string, id *age.X25519Identity, password string) error {
	rcp, err := age.NewScryptRecipient(password)
	if err != nil {
		return err
	}
	if m.kdfWork > 0 {
		rcp.SetWorkFactor(m.kdfWork)
	}
	var out bytes.Buffer
	w, err := age.Encrypt(&out, rcp)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(id.String())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return m.raw().Set(vault.ProtectKey(name), out.Bytes())
}

// the vault entries holding a profile's logins
func (m *Manager) secretKeys(name string) []string {
	var keys []string
	for _, a := range m.all {
		cand := []string{vault.ProfileKey(name, a.ID()), vault.LockKey(name, a.ID())}
		if m.Cfg.Active == name {
			cand = append(cand, vault.LockKey(vault.LockAll, a.ID()))
		}
		for _, k := range cand {
			if _, err := m.raw().Get(k); err == nil {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

// sets or changes the password, old is only needed for a change
func (m *Manager) SetPassword(ctx context.Context, name, old, next string, dryRun bool) ([]adapter.Change, error) {
	p, err := m.Cfg.Profile(name)
	if err != nil {
		return nil, err
	}
	if len(next) < MinPasswordLen {
		return nil, fmt.Errorf("the password needs at least %d characters", MinPasswordLen)
	}
	changes := []adapter.Change{{Kind: "keyring", Target: vault.ProtectKey(name), Action: "write"}}
	if p.Protected() {
		if err := m.unsealWith(name, old); err != nil {
			return nil, err
		}
		if dryRun {
			return changes, nil
		}
		if err := m.storeKey(name, m.keys[name], next); err != nil {
			return nil, err
		}
		m.Log("password change %s", name)
		return changes, nil
	}

	keys := m.secretKeys(name)
	for _, k := range keys {
		changes = append(changes, adapter.Change{Kind: "keyring", Target: k, Action: "encrypt"})
	}
	changes = append(changes,
		adapter.Change{Kind: "file", Target: m.Paths.ConfigFile() + " (profiles." + name + ")", Action: "encrypt"},
		adapter.Change{Kind: "file", Target: m.Paths.BackupDir() + " (backups of " + name + ")", Action: "encrypt"})
	if dryRun {
		return changes, nil
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, err
	}
	if err := m.storeKey(name, id, next); err != nil {
		return nil, err
	}
	p.SetProtection(id.Recipient().String())
	m.keys[name] = id
	// config first, so a failure below leaves readable plain entries instead of entries nobody can open
	if err := m.Save(); err != nil {
		return nil, err
	}
	var errs []error
	for _, k := range keys {
		if b, err := m.raw().Get(k); err == nil {
			errs = append(errs, m.Env.Vault.Set(k, b))
		}
	}
	errs = append(errs, m.Backups.SealFor(name))
	if m.Cfg.Active != name {
		errs = append(errs, m.stashGitHub(ctx, p), m.closeProfile(p))
	}
	errs = append(errs, m.Save())
	m.Log("password set %s", name)
	return changes, errors.Join(errs...)
}

func (m *Manager) RemovePassword(ctx context.Context, name, password string, dryRun bool) ([]adapter.Change, error) {
	p, err := m.Cfg.Profile(name)
	if err != nil {
		return nil, err
	}
	if !p.Protected() {
		return nil, fmt.Errorf("profile %q has no password", name)
	}
	if err := m.unsealWith(name, password); err != nil {
		return nil, err
	}
	changes := []adapter.Change{{Kind: "keyring", Target: vault.ProtectKey(name), Action: "delete"}}
	for _, k := range m.secretKeys(name) {
		changes = append(changes, adapter.Change{Kind: "keyring", Target: k, Action: "decrypt"})
	}
	changes = append(changes,
		adapter.Change{Kind: "file", Target: m.Paths.ConfigFile() + " (profiles." + name + ")", Action: "decrypt"},
		adapter.Change{Kind: "file", Target: m.Paths.BackupDir() + " (backups of " + name + ")", Action: "decrypt"})
	if dryRun {
		return changes, nil
	}
	if err := m.Backups.UnsealFor(name); err != nil {
		return nil, fmt.Errorf("backups: %w (the password is still set)", err)
	}
	var errs []error
	if m.Cfg.Active != name {
		_, err := m.unstashGitHub(ctx, p, false)
		errs = append(errs, err)
	}
	vals := map[string][]byte{}
	for _, k := range m.secretKeys(name) {
		b, err := m.Env.Vault.Get(k)
		if err != nil {
			return nil, fmt.Errorf("%s: %w (the password is still set)", k, err)
		}
		vals[k] = b
	}
	if err := p.ClearProtection(); err != nil {
		return nil, err
	}
	for k, b := range vals {
		errs = append(errs, m.raw().Set(k, b))
	}
	if err := m.Save(); err != nil {
		return nil, err
	}
	delete(m.keys, name)
	errs = append(errs, m.raw().Delete(vault.ProtectKey(name)))
	_, err = m.SyncGit(false)
	errs = append(errs, err)
	m.Log("password remove %s", name)
	return changes, errors.Join(errs...)
}

// encrypts a profile again and takes its rules and git identity out of ~/.gitconfig
func (m *Manager) closeProfile(p *config.Profile) error {
	if err := p.Close(); err != nil {
		return err
	}
	_, err := m.SyncGit(false)
	return err
}

// after switching away from a protected profile, its gh login and git rules must not stay usable
func (m *Manager) leaveProtected(ctx context.Context, from *config.Profile, ads []adapter.Adapter) error {
	var errs []error
	for _, a := range ads {
		if a.ID() == "github" {
			errs = append(errs, m.stashGitHub(ctx, from))
		}
	}
	errs = append(errs, m.closeProfile(from))
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("switched, but %s isn't fully hidden yet: %w", from.Name, err)
	}
	return nil
}

func (m *Manager) gitHub() adapter.Locker {
	for _, a := range m.Adapters {
		if l, ok := a.(adapter.Locker); ok && a.ID() == "github" {
			return l
		}
	}
	return nil
}

func gitHubStashKey(profile string) string { return vault.ProfileKey(profile, "github") }

func (m *Manager) hasStash(profile string) bool {
	_, err := m.raw().Get(gitHubStashKey(profile))
	return err == nil
}

// gh keeps every account it knows, so a protected profile's account is logged out and its token kept encrypted
func (m *Manager) stashGitHub(ctx context.Context, p *config.Profile) error {
	l, acct := m.gitHub(), p.Accounts["github"]
	if l == nil || acct == nil {
		return nil
	}
	_, _, err := l.Lock(ctx, m.Env, gitHubStashKey(p.Name), acct.Label, false)
	return err
}

func (m *Manager) unstashGitHub(ctx context.Context, p *config.Profile, dryRun bool) ([]adapter.Change, error) {
	l := m.gitHub()
	if l == nil || !m.hasStash(p.Name) {
		return nil, nil
	}
	_, ch, err := l.Unlock(ctx, m.Env, gitHubStashKey(p.Name), dryRun)
	return ch, err
}

// claude and codex sessions of cream run hold a plain copy of the token, a protected profile doesn't keep it
func (m *Manager) dropSessionTokens(profile string) {
	for _, f := range []string{filepath.Join("claude", ".credentials.json"), filepath.Join("codex", "auth.json")} {
		_ = os.Remove(filepath.Join(m.Paths.CreamHome, "sessions", profile, f))
	}
}

// values of a protected profile's vault entries, encrypted to its key, the hash lets syncOut skip unchanged tokens
type sealVault struct {
	vault.Vault
	m *Manager
}

const sealMagic = "cream-sealed1:"

// the profile whose password guards a key, lock entries of everything belong to the active profile
func (s *sealVault) owner(key string) *config.Profile {
	kind, rest, ok := strings.Cut(key, ":")
	if !ok || (kind != "profile" && kind != "lock") {
		return nil
	}
	name, _, ok := strings.Cut(rest, ":")
	if !ok {
		return nil
	}
	if kind == "lock" && name == vault.LockAll {
		name = s.m.Cfg.Active
	}
	p := s.m.Cfg.Profiles[name]
	if p == nil || !p.Protected() {
		return nil
	}
	return p
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func (s *sealVault) Set(key string, value []byte) error {
	p := s.owner(key)
	if p == nil {
		return s.Vault.Set(key, value)
	}
	rcp, err := age.ParseX25519Recipient(p.Password.Recipient)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	out.WriteString(sealMagic + digest(value) + "\n")
	w, err := age.Encrypt(&out, rcp)
	if err != nil {
		return err
	}
	if _, err := w.Write(value); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return s.Vault.Set(key, out.Bytes())
}

func (s *sealVault) Get(key string) ([]byte, error) {
	b, err := s.Vault.Get(key)
	if err != nil || !bytes.HasPrefix(b, []byte(sealMagic)) {
		return b, err
	}
	p := s.owner(key)
	if p == nil {
		return nil, fmt.Errorf("vault entry %q is encrypted for a profile without a password", key)
	}
	id := s.m.keys[p.Name]
	if id == nil {
		return nil, PasswordRequired{p.Name}
	}
	_, body, _ := bytes.Cut(b, []byte("\n"))
	r, err := age.Decrypt(bytes.NewReader(body), id)
	if err != nil {
		return nil, fmt.Errorf("vault entry %q: %w", key, err)
	}
	return io.ReadAll(r)
}

// whether key already holds value, without the password
func (s *sealVault) same(key string, value []byte) bool {
	b, err := s.Vault.Get(key)
	if err != nil {
		return false
	}
	if rest, ok := bytes.CutPrefix(b, []byte(sealMagic)); ok {
		sum, _, _ := bytes.Cut(rest, []byte("\n"))
		return string(sum) == digest(value)
	}
	return bytes.Equal(b, value)
}
