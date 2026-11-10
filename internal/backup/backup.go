// snapshots are age encrypted to an x25519 identity that lives in the vault
package backup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/gdamigos/cream/internal/fsutil"
	"github.com/gdamigos/cream/internal/vault"
)

const Keep = 20

type FileEntry struct {
	Path    string      `json:"path"`
	Exists  bool        `json:"exists"`
	Mode    os.FileMode `json:"mode,omitempty"`
	Content []byte      `json:"content,omitempty"`
}

type Snapshot struct {
	ID      string                       `json:"id"`
	Created time.Time                    `json:"created"`
	Reason  string                       `json:"reason"`
	From    string                       `json:"from"`
	To      string                       `json:"to"`
	Files   []FileEntry                  `json:"files"`
	Extra   map[string]map[string][]byte `json:"extra,omitempty"` // keyed by adapter id
	// a password protected profile whose live logins are in here, only its key opens the backup
	Owner string `json:"owner,omitempty"`
}

type Info struct {
	ID      string
	Created time.Time
	Path    string
	Reason  string
	Owner   string
}

type Store struct {
	Dir   string
	Vault vault.Vault
	// set by core for password protected profiles
	Recipient func(owner string) (age.Recipient, error)
	Identity  func(owner string) (age.Identity, error)
}

func (s *Store) identity() (*age.X25519Identity, error) {
	b, err := s.Vault.Get(vault.BackupIdentityKey)
	if err == nil {
		return age.ParseX25519Identity(strings.TrimSpace(string(b)))
	}
	if !errors.Is(err, vault.ErrNotFound) {
		return nil, err
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, err
	}
	if err := s.Vault.Set(vault.BackupIdentityKey, []byte(id.String())); err != nil {
		return nil, err
	}
	return id, nil
}

func Capture(paths []string) (*Snapshot, error) {
	now := time.Now().UTC()
	snap := &Snapshot{ID: now.Format("20060102T150405.000000000Z"), Created: now}
	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true
		e := FileEntry{Path: p}
		b, ok, err := fsutil.ReadFileIfExists(p)
		if err != nil {
			return nil, fmt.Errorf("backup %s: %w", p, err)
		}
		if ok {
			st, err := os.Stat(p)
			if err != nil {
				return nil, err
			}
			e.Exists, e.Mode, e.Content = true, st.Mode().Perm(), b
		}
		snap.Files = append(snap.Files, e)
	}
	return snap, nil
}

func (s *Store) recipient(owner string) (age.Recipient, error) {
	if owner != "" {
		if s.Recipient == nil {
			return nil, fmt.Errorf("no key for %q's backups", owner)
		}
		return s.Recipient(owner)
	}
	id, err := s.identity()
	if err != nil {
		return nil, err
	}
	return id.Recipient(), nil
}

func (s *Store) identityFor(owner string) (age.Identity, error) {
	if owner != "" {
		if s.Identity == nil {
			return nil, fmt.Errorf("no key for %q's backups", owner)
		}
		return s.Identity(owner)
	}
	return s.identity()
}

// <id>_<reason>[~<owner>].age, the owner says whose password opens it
func fileName(id, reason, owner string) string {
	if owner != "" {
		return id + "_" + reason + "~" + owner + ".age"
	}
	return id + "_" + reason + ".age"
}

func (s *Store) Save(snap *Snapshot) (string, error) {
	path, err := s.write(snap)
	if err != nil {
		return "", err
	}
	s.prune()
	return path, nil
}

func (s *Store) write(snap *Snapshot) (string, error) {
	rcp, err := s.recipient(snap.Owner)
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(snap)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	w, err := age.Encrypt(&out, rcp)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(plain); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(s.Dir, fileName(snap.ID, sanitize(snap.Reason), snap.Owner))
	if err := fsutil.WriteFileAtomic(path, out.Bytes(), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "backup"
	}
	return b.String()
}

func (s *Store) List() ([]Info, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".age") {
			continue
		}
		base := strings.TrimSuffix(name, ".age")
		id, rest, _ := strings.Cut(base, "_")
		reason, owner, _ := strings.Cut(rest, "~")
		t, err := time.Parse("20060102T150405.000000000Z", id)
		if err != nil {
			continue
		}
		out = append(out, Info{ID: id, Created: t, Path: filepath.Join(s.Dir, name), Reason: reason, Owner: owner})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (s *Store) prune() {
	list, err := s.List()
	if err != nil {
		return
	}
	for i := Keep; i < len(list); i++ {
		_ = os.Remove(list[i].Path)
	}
}

func (s *Store) Load(id string) (*Snapshot, error) {
	list, err := s.List()
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, errors.New("no backups yet")
	}
	var info *Info
	if id == "" || id == "latest" {
		info = &list[0]
	} else {
		for i := range list {
			if list[i].ID == id || strings.HasPrefix(list[i].ID, id) {
				info = &list[i]
				break
			}
		}
	}
	if info == nil {
		return nil, fmt.Errorf("backup %q not found", id)
	}
	return s.load(*info)
}

func (s *Store) load(info Info) (*Snapshot, error) {
	ident, err := s.identityFor(info.Owner)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(info.Path)
	if err != nil {
		return nil, err
	}
	r, err := age.Decrypt(bytes.NewReader(raw), ident)
	if err != nil {
		return nil, fmt.Errorf("decrypt backup: %w", err)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(plain, &snap); err != nil {
		return nil, err
	}
	snap.Owner = info.Owner
	return &snap, nil
}

// re-encrypts the backups holding owner's live logins to owner's key, after a password is set
func (s *Store) SealFor(owner string) error {
	return s.rewrap("", func(snap *Snapshot) bool { return snap.From == owner }, owner)
}

// re-encrypts owner's backups to the shared backup key, before its password is removed
func (s *Store) UnsealFor(owner string) error {
	return s.rewrap(owner, func(*Snapshot) bool { return true }, "")
}

// only backups currently sealed for from are opened, others may need a password we don't have
func (s *Store) rewrap(from string, match func(*Snapshot) bool, to string) error {
	list, err := s.List()
	if err != nil {
		return err
	}
	var errs []error
	for _, info := range list {
		if info.Owner != from {
			continue
		}
		snap, err := s.load(info)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !match(snap) {
			continue
		}
		snap.Owner = to
		path, err := s.write(snap)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if path != info.Path {
			errs = append(errs, os.Remove(info.Path))
		}
	}
	return errors.Join(errs...)
}

// files that did not exist at snapshot time get deleted
func RestoreFiles(snap *Snapshot) error {
	var errs []error
	for _, f := range snap.Files {
		if !f.Exists {
			if err := fsutil.RemoveIfExists(f.Path); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		mode := f.Mode
		if mode == 0 {
			mode = 0o600
		}
		if err := fsutil.WriteFileAtomic(f.Path, f.Content, mode); err != nil {
			errs = append(errs, err)
			continue
		}
		_ = os.Chmod(f.Path, mode)
	}
	return errors.Join(errs...)
}
