package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"filippo.io/age"

	"github.com/gdamigos/cream/internal/fsutil"
)

// fallback when there is no keyring, the whole map is age/scrypt encrypted and plaintext never hits disk
type File struct {
	path       string
	passphrase string
	workFactor int

	mu     sync.Mutex
	loaded bool
	stamp  string // size and mtime of the file as last read or written
	m      map[string][]byte
}

func NewFile(path, passphrase string, workFactor int) (*File, error) {
	if passphrase == "" {
		return nil, errors.New("the encrypted file vault needs a passphrase (set CREAM_VAULT_PASSPHRASE or run interactively)")
	}
	return &File{path: path, passphrase: passphrase, workFactor: workFactor}, nil
}

func (*File) Backend() string { return "file" }

// reads the file again when another cream process wrote it, so a write never drops its entries
func (f *File) load() error {
	if f.loaded && fsutil.Stamp(f.path) == f.stamp {
		return nil
	}
	f.m = map[string][]byte{}
	// stamped before reading for the same reason as config.toml in core.Open
	stamp := fsutil.Stamp(f.path)
	raw, ok, err := fsutil.ReadFileIfExists(f.path)
	if err != nil {
		return err
	}
	if ok {
		id, err := age.NewScryptIdentity(f.passphrase)
		if err != nil {
			return err
		}
		r, err := age.Decrypt(bytes.NewReader(raw), id)
		if err != nil {
			return fmt.Errorf("unlock vault %s: wrong passphrase or corrupt file: %w", f.path, err)
		}
		plain, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(plain, &f.m); err != nil {
			return fmt.Errorf("vault %s: %w", f.path, err)
		}
	}
	f.loaded, f.stamp = true, stamp
	return nil
}

func (f *File) save() error {
	plain, err := json.Marshal(f.m)
	if err != nil {
		return err
	}
	rcp, err := age.NewScryptRecipient(f.passphrase)
	if err != nil {
		return err
	}
	if f.workFactor > 0 {
		rcp.SetWorkFactor(f.workFactor)
	}
	var out bytes.Buffer
	w, err := age.Encrypt(&out, rcp)
	if err != nil {
		return err
	}
	if _, err := w.Write(plain); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(f.path, out.Bytes(), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(f.path, 0o600); err != nil {
		return err
	}
	f.stamp = fsutil.Stamp(f.path)
	return nil
}

func (f *File) Get(key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return nil, err
	}
	b, ok := f.m[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), b...), nil
}

func (f *File) Set(key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return err
	}
	f.m[key] = append([]byte(nil), value...)
	return f.save()
}

func (f *File) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return err
	}
	if _, ok := f.m[key]; !ok {
		return ErrNotFound
	}
	delete(f.m, key)
	return f.save()
}

func (f *File) Unlock() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.load()
}

type OpenOptions struct {
	Backend    string // auto, keyring or file
	FilePath   string
	Passphrase func() (string, error) // only called for the file vault
}

func Open(o OpenOptions) (Vault, error) {
	useFile := false
	switch o.Backend {
	case "", "auto":
		useFile = !KeyringAvailable()
	case "keyring":
	case "file":
		useFile = true
	default:
		return nil, fmt.Errorf("unknown vault backend %q (want auto, keyring or file)", o.Backend)
	}
	if !useFile {
		return Keyring{}, nil
	}
	pass := os.Getenv("CREAM_VAULT_PASSPHRASE")
	if pass == "" && o.Passphrase != nil {
		var err error
		if pass, err = o.Passphrase(); err != nil {
			return nil, err
		}
	}
	fv, err := NewFile(o.FilePath, pass, 0)
	if err != nil {
		return nil, err
	}
	if err := fv.Unlock(); err != nil {
		return nil, err
	}
	return fv, nil
}
