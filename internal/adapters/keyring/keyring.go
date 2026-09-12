package keyring

import (
	"errors"
	"fmt"
	"os"
	"sync"

	zkr "github.com/zalando/go-keyring"

	"github.com/nord-launcher/launcher/internal/core/ports"
)

var ErrKeyNotFound = errors.New("key not found in keyring")

// InMemKeyring provides thread-safe credential storage for tests and fallback.
type InMemKeyring struct {
	mu    sync.RWMutex
	store map[string]string
}

func NewMemoryKeyring() ports.Keyring {
	return &InMemKeyring{
		store: make(map[string]string),
	}
}

func key(service, user string) string {
	return service + "::" + user
}

func (k *InMemKeyring) Get(service, user string) (string, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	val, ok := k.store[key(service, user)]
	if !ok {
		return "", ErrKeyNotFound
	}
	return val, nil
}

func (k *InMemKeyring) Set(service, user, password string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.store[key(service, user)] = password
	return nil
}

func (k *InMemKeyring) Delete(service, user string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.store, key(service, user))
	return nil
}

// SystemKeyring provides persistent credential storage via host OS Credential Manager
// (Windows Credential Manager / Linux Secret Service) with explicit warning fallback.
type SystemKeyring struct {
	fallback    ports.Keyring
	useFallback bool
}

// NewSystemKeyring initializes the OS-native keyring or transparently falls back to InMemKeyring with a warning if unavailable.
func NewSystemKeyring() ports.Keyring {
	mem := NewMemoryKeyring()
	// Probe system keyring with an ephemeral test probe
	probeSvc := "nord-launcher-probe"
	probeUser := "probe-user"
	probeVal := "probe-val"

	err := zkr.Set(probeSvc, probeUser, probeVal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] System keyring unavailable (%v), falling back to InMemKeyring. Credentials will not persist across restarts.\n", err)
		return mem
	}
	// Clean up probe
	_ = zkr.Delete(probeSvc, probeUser) // slop:ok best-effort probe secret cleanup

	return &SystemKeyring{
		fallback:    mem,
		useFallback: false,
	}
}

func (s *SystemKeyring) Get(service, user string) (string, error) {
	if s.useFallback {
		return s.fallback.Get(service, user)
	}
	val, err := zkr.Get(service, user)
	if err != nil {
		if errors.Is(err, zkr.ErrNotFound) {
			return "", ErrKeyNotFound
		}
		return "", err
	}
	return val, nil
}

func (s *SystemKeyring) Set(service, user, password string) error {
	if s.useFallback {
		return s.fallback.Set(service, user, password)
	}
	return zkr.Set(service, user, password)
}

func (s *SystemKeyring) Delete(service, user string) error {
	if s.useFallback {
		return s.fallback.Delete(service, user)
	}
	err := zkr.Delete(service, user)
	if err != nil && errors.Is(err, zkr.ErrNotFound) {
		return nil
	}
	return err
}
