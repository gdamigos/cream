package vault

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const Service = "cream"

var ErrNotFound = errors.New("secret not found in vault")

type Vault interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
	Backend() string
}

// stays under the windows credential manager limit (2560 bytes) and the macos security -i line limit
const chunkSize = 1500

// values are base64, stored as "v1:<data>" or as "v1c:<n>" with the data in key#0..key#n-1
type Keyring struct{}

func (Keyring) Backend() string { return "keyring" }

func (Keyring) Get(key string) ([]byte, error) {
	head, err := keyring.Get(Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasPrefix(head, "v1:"):
		return base64.StdEncoding.DecodeString(head[3:])
	case strings.HasPrefix(head, "v1c:"):
		n, err := strconv.Atoi(head[4:])
		if err != nil || n < 1 || n > 1000 {
			return nil, fmt.Errorf("corrupt vault entry %q", key)
		}
		var sb strings.Builder
		for i := 0; i < n; i++ {
			part, err := keyring.Get(Service, chunkKey(key, i))
			if err != nil {
				return nil, fmt.Errorf("vault entry %q chunk %d: %w", key, i, err)
			}
			sb.WriteString(part)
		}
		return base64.StdEncoding.DecodeString(sb.String())
	default:
		return nil, fmt.Errorf("unrecognised vault entry format for %q", key)
	}
}

func (k Keyring) Set(key string, value []byte) error {
	_ = k.Delete(key)
	enc := base64.StdEncoding.EncodeToString(value)
	if len(enc) <= chunkSize {
		return keyring.Set(Service, key, "v1:"+enc)
	}
	n := 0
	for start := 0; start < len(enc); start += chunkSize {
		end := min(start+chunkSize, len(enc))
		if err := keyring.Set(Service, chunkKey(key, n), enc[start:end]); err != nil {
			return err
		}
		n++
	}
	return keyring.Set(Service, key, "v1c:"+strconv.Itoa(n))
}

func (Keyring) Delete(key string) error {
	head, err := keyring.Get(Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if strings.HasPrefix(head, "v1c:") {
		if n, err := strconv.Atoi(head[4:]); err == nil {
			for i := 0; i < n && i < 1000; i++ {
				_ = keyring.Delete(Service, chunkKey(key, i))
			}
		}
	}
	return keyring.Delete(Service, key)
}

func chunkKey(key string, i int) string { return key + "#" + strconv.Itoa(i) }

func KeyringAvailable() bool {
	const probe = "cream:probe"
	if err := keyring.Set(Service, probe, "ok"); err != nil {
		return false
	}
	v, err := keyring.Get(Service, probe)
	_ = keyring.Delete(Service, probe)
	return err == nil && v == "ok"
}

type Memory struct {
	mu sync.Mutex
	m  map[string][]byte
}

func NewMemory() *Memory { return &Memory{m: map[string][]byte{}} }

func (*Memory) Backend() string { return "memory" }

func (v *Memory) Get(key string) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	b, ok := v.m[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), b...), nil
}

func (v *Memory) Set(key string, value []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m[key] = append([]byte(nil), value...)
	return nil
}

func (v *Memory) Delete(key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.m[key]; !ok {
		return ErrNotFound
	}
	delete(v.m, key)
	return nil
}

func (v *Memory) Keys() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]string, 0, len(v.m))
	for k := range v.m {
		out = append(out, k)
	}
	return out
}

func ProfileKey(profile, adapter string) string {
	return "profile:" + profile + ":" + adapter
}

// scope is a profile name, or LockAll for cream lock without a profile
func LockKey(scope, adapter string) string {
	return "lock:" + scope + ":" + adapter
}

// not a valid profile name, so it can't collide with one
const LockAll = "*"

const BackupIdentityKey = "cream:backup-identity"

// a password protected profile's age identity, scrypt encrypted with the password
func ProtectKey(profile string) string { return "protect:" + profile }

func DeleteProfile(v Vault, profile string, adapters []string) {
	for _, a := range adapters {
		_ = v.Delete(ProfileKey(profile, a))
	}
}
