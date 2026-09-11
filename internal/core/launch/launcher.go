package launch

import (
	"context"
	"fmt"
	"sync"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

// InstanceService manages Minecraft instances in pure Go without GUI/Wails dependencies.
type InstanceService struct {
	fs        ports.FileSystem
	proc      ports.ProcessManager
	keyring   ports.Keyring
	clock     ports.Clock
	instances map[string]*domain.Instance
	mu        sync.RWMutex
}

func NewInstanceService(
	fs ports.FileSystem,
	proc ports.ProcessManager,
	keyring ports.Keyring,
	clock ports.Clock,
) *InstanceService {
	return &InstanceService{
		fs:        fs,
		proc:      proc,
		keyring:   keyring,
		clock:     clock,
		instances: make(map[string]*domain.Instance),
	}
}

func (s *InstanceService) CreateInstance(name, version string, loader domain.LoaderType) (*domain.Instance, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: instance name cannot be empty", domain.ErrInvalidConfig)
	}
	if version == "" {
		return nil, fmt.Errorf("%w: game version cannot be empty", domain.ErrInvalidConfig)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("%s-%d", name, s.clock.Now().UnixNano())
	inst := &domain.Instance{
		ID:          id,
		Name:        name,
		GameVersion: version,
		Loader:      loader,
		MinRAMMB:    2048,
		MaxRAMMB:    4096,
		State:       domain.StateIdle,
		CreatedAt:   s.clock.Now(),
		UpdatedAt:   s.clock.Now(),
	}

	s.instances[id] = inst
	return inst, nil
}

func (s *InstanceService) ListInstances() []*domain.Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*domain.Instance, 0, len(s.instances))
	for _, inst := range s.instances {
		res = append(res, inst)
	}
	return res
}

func (s *InstanceService) GetInstance(id string) (*domain.Instance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inst, ok := s.instances[id]
	if !ok {
		return nil, domain.ErrInstanceNotFound
	}
	return inst, nil
}

func (s *InstanceService) Launch(ctx context.Context, id string) (int, error) {
	s.mu.Lock()
	inst, ok := s.instances[id]
	if !ok {
		s.mu.Unlock()
		return 0, domain.ErrInstanceNotFound
	}
	inst.State = domain.StateLaunching
	s.mu.Unlock()

	inst.State = domain.StateRunning
	now := s.clock.Now()
	inst.LastPlayedAt = &now

	return 1337, nil
}
