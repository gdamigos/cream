package config

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"filippo.io/age"
)

// a password protected profile keeps everything but its name encrypted to its own age key
// the private key lives in the vault, encrypted with the password
type Protection struct {
	Recipient string `toml:"recipient"`        // age x25519 public key
	Sealed    string `toml:"sealed,omitempty"` // base64 age ciphertext of the hidden fields
}

type hidden struct {
	Description string              `json:"description,omitempty"`
	Git         GitIdentity         `json:"git"`
	Rules       []Rule              `json:"rules,omitempty"`
	Accounts    map[string]*Account `json:"accounts,omitempty"`
}

func (p *Profile) Protected() bool { return p.Password != nil }

// protected and not decrypted, so description, git, rules and accounts are empty
func (p *Profile) Sealed() bool { return p.Password != nil && !p.open }

func (p *Profile) hasPlain() bool {
	return p.Description != "" || !p.Git.IsZero() || len(p.Rules) > 0 || len(p.Accounts) > 0
}

// turns protection on, the profile stays readable in memory until Close
func (p *Profile) SetProtection(recipient string) {
	p.Password = &Protection{Recipient: recipient}
	p.open = true
}

// turns protection off, needs an open profile so nothing is lost
func (p *Profile) ClearProtection() error {
	if p.Sealed() {
		return fmt.Errorf("profile %q is still encrypted", p.Name)
	}
	p.Password, p.open = nil, false
	return nil
}

// decrypts the hidden fields with the profile's identity
func (p *Profile) Open(id age.Identity) error {
	if !p.Sealed() {
		return nil
	}
	var h hidden
	if p.Password.Sealed != "" {
		raw, err := base64.StdEncoding.DecodeString(p.Password.Sealed)
		if err != nil {
			return fmt.Errorf("profile %q: corrupt encrypted data: %w", p.Name, err)
		}
		r, err := age.Decrypt(bytes.NewReader(raw), id)
		if err != nil {
			return fmt.Errorf("profile %q: decrypt: %w", p.Name, err)
		}
		plain, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(plain, &h); err != nil {
			return fmt.Errorf("profile %q: corrupt encrypted data: %w", p.Name, err)
		}
	}
	p.Description, p.Git, p.Rules, p.Accounts = h.Description, h.Git, h.Rules, h.Accounts
	if p.Accounts == nil {
		p.Accounts = map[string]*Account{}
	}
	p.open = true
	return nil
}

// encrypts the open fields to the recipient, needs no password
func (p *Profile) seal() error {
	if !p.Protected() || !p.open {
		return nil
	}
	plain, err := json.Marshal(hidden{p.Description, p.Git, p.Rules, p.Accounts})
	if err != nil {
		return err
	}
	rcp, err := age.ParseX25519Recipient(p.Password.Recipient)
	if err != nil {
		return fmt.Errorf("profile %q: %w", p.Name, err)
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
	p.Password.Sealed = base64.StdEncoding.EncodeToString(out.Bytes())
	return nil
}

// seals and forgets the decrypted fields
func (p *Profile) Close() error {
	if !p.Protected() || !p.open {
		return nil
	}
	if err := p.seal(); err != nil {
		return err
	}
	p.Description, p.Git, p.Rules, p.Accounts = "", GitIdentity{}, nil, map[string]*Account{}
	p.open = false
	return nil
}

// what Save writes: the active profile stays readable because its logins and git rules are live anyway
func (c *Config) forDisk() (*Config, error) {
	out := *c
	out.Profiles = make(map[string]*Profile, len(c.Profiles))
	for name, p := range c.Profiles {
		if p.Protected() {
			if err := p.seal(); err != nil {
				return nil, err
			}
			if name != c.Active || !p.open {
				cp := *p
				cp.Description, cp.Git, cp.Rules, cp.Accounts = "", GitIdentity{}, nil, nil
				p = &cp
			}
		}
		out.Profiles[name] = p
	}
	return &out, nil
}
