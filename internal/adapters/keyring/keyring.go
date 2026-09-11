package keyring

import (
	"errors"
	"sync"

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
