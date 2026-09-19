package launch

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/java"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

// SessionRefresher provides Microsoft session token refresh.
type SessionRefresher interface {
	RefreshSession(ctx context.Context, uuid string) (*domain.Account, error)
}

// InstanceService manages Minecraft instances in pure Go without GUI/Wails dependencies.
type InstanceService struct {
	repo             ports.InstanceRepository
	fs               ports.FileSystem
	proc             ports.ProcessManager
	keyring          ports.Keyring
	clock            ports.Clock
	provisioner      ports.GameProvisioner
	java             ports.JavaDetector
	accRepo          ports.AccountRepository
	activeAccount    *domain.Account
	sessionRefresher SessionRefresher
	onCrash          func(instanceID string, report *CrashReport)
	instances        map[string]*domain.Instance
	mu               sync.RWMutex
}

func NewInstanceService(
	repo ports.InstanceRepository,
	fs ports.FileSystem,
	proc ports.ProcessManager,
	keyring ports.Keyring,
	clock ports.Clock,
) *InstanceService {
	return &InstanceService{
		repo:      repo,
		fs:        fs,
		proc:      proc,
		keyring:   keyring,
		clock:     clock,
		instances: make(map[string]*domain.Instance),
	}
}

func (s *InstanceService) SetProvisioner(p ports.GameProvisioner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provisioner = p
}

func (s *InstanceService) SetJavaDetector(j ports.JavaDetector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.java = j
}

func (s *InstanceService) SetAccountRepository(a ports.AccountRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accRepo = a
}

func (s *InstanceService) SetActiveAccount(acc *domain.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeAccount = acc
}

func (s *InstanceService) SetSessionRefresher(r SessionRefresher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionRefresher = r
}

func (s *InstanceService) SetOnCrash(cb func(instanceID string, report *CrashReport)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCrash = cb
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

	if s.repo != nil {
		if err := s.repo.Save(context.Background(), inst); err != nil {
			return nil, fmt.Errorf("persist instance to storage: %w", err)
		}
	}

	return inst, nil
}

func (s *InstanceService) ListInstances() []*domain.Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.repo != nil {
		list, err := s.repo.ListAll(context.Background())
		if err == nil {
			return list
		}
	}

	res := make([]*domain.Instance, 0, len(s.instances))
	for _, inst := range s.instances {
		res = append(res, inst)
	}
	return res
}

func (s *InstanceService) GetInstance(id string) (*domain.Instance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.repo != nil {
		inst, err := s.repo.GetByID(context.Background(), id)
		if err == nil {
			return inst, nil
		}
	}

	inst, ok := s.instances[id]
	if !ok {
		return nil, domain.ErrInstanceNotFound
	}
	copyInst := *inst
	return &copyInst, nil
}

func (s *InstanceService) Launch(ctx context.Context, id string) (int, error) {
	s.mu.RLock()
	inst, ok := s.instances[id]
	s.mu.RUnlock()

	if !ok && s.repo != nil {
		var err error
		inst, err = s.repo.GetByID(ctx, id)
		if err != nil || inst == nil {
			return 0, domain.ErrInstanceNotFound
		}
		s.mu.Lock()
		s.instances[id] = inst
		s.mu.Unlock()
	} else if !ok {
		return 0, domain.ErrInstanceNotFound
	}

	// 1. Resolve active account
	var acc *domain.Account
	if s.accRepo != nil {
		var err error
		acc, err = s.accRepo.GetActive(ctx)
		if err != nil {
			acc = nil
		}
	}
	if acc == nil {
		s.mu.RLock()
		acc = s.activeAccount
		s.mu.RUnlock()
	}
	if acc == nil {
		return 0, domain.ErrNoActiveAccount
	}

	// 2. Microsoft token freshness check (V1)
	if acc.Type == domain.AccountMicrosoft && !acc.ExpiresAt.IsZero() {
		if s.clock.Now().After(acc.ExpiresAt.Add(-5*time.Minute)) && s.sessionRefresher != nil {
			refreshed, err := s.sessionRefresher.RefreshSession(ctx, acc.UUID)
			if err == nil && refreshed != nil {
				acc = refreshed
			}
		}
	}

	// 4. Resolve Java executable (V2)
	javaExec := inst.JavaPath
	if javaExec == "" && s.java != nil {
		reqMajor, _ := java.ResolveJavaMajor(inst.GameVersion)
		if installs, err := s.java.DetectInstallations(ctx); err == nil {
			for _, install := range installs {
				if install.MajorVersion == reqMajor {
					javaExec = install.Path
					break
				}
			}
			if javaExec == "" && len(installs) > 0 {
				javaExec = installs[0].Path
			}
		}
	}
	if javaExec == "" {
		javaExec = "java"
	}

	// 5. Game Provisioning (B1)
	var cfg *domain.LaunchConfig
	if s.provisioner != nil {
		s.mu.Lock()
		inst.State = domain.StateDownloading
		if s.repo != nil {
			if err := s.repo.UpdateState(ctx, id, domain.StateDownloading); err != nil {
				log.Printf("failed to update instance state to downloading: %v", err)
			}
		}
		s.mu.Unlock()

		var provErr error
		cfg, provErr = s.provisioner.Provision(ctx, inst, acc)
		if provErr != nil {
			s.mu.Lock()
			inst.State = domain.StateCrashed
			if s.repo != nil {
				if err := s.repo.UpdateState(ctx, id, domain.StateCrashed); err != nil {
					log.Printf("failed to update instance state to crashed: %v", err)
				}
			}
			s.mu.Unlock()
			return 0, fmt.Errorf("provision game: %w", provErr)
		}
	} else {
		// Fallback for tests running without provisioner
		gameDir := filepath.Join(".", "instances", inst.ID)
		if inst.Name != "" {
			gameDir = filepath.Join(".", "instances", inst.Name)
		}
		cfg = &domain.LaunchConfig{
			Instance: inst,
			Account:  acc,
			VersionMeta: &domain.VersionJSON{
				ID:                 inst.GameVersion,
				MainClass:          "net.minecraft.client.main.Main",
				MinecraftArguments: "--username ${auth_player_name} --version ${version_name} --gameDir ${game_directory} --uuid ${auth_uuid} --accessToken ${auth_access_token} --userType ${user_type}",
			},
			GameDir: gameDir,
		}
	}

	// 6. Launch with supervisor
	handle, supervisor, err := s.LaunchWithSupervisor(ctx, *cfg, javaExec)
	if err != nil {
		return 0, err
	}

	// 7. Supervise in background (C2)
	go MonitorProcess(handle, inst, supervisor, s.onCrash, s.repo, &s.mu)

	return handle.PID(), nil
}

// MonitorProcess supervises an active process until exit, updates instance state,
// and invokes crash diagnostics if exit code is non-zero.
func MonitorProcess(
	handle ports.ProcessHandle,
	inst *domain.Instance,
	supervisor *LogSupervisor,
	onCrash func(string, *CrashReport),
	repo ports.InstanceRepository,
	mu *sync.RWMutex,
) {
	exitCode, _ := handle.Wait() // errcheck:ok wait error handled via exit code diagnostics

	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}

	if exitCode == 0 {
		inst.State = domain.StateIdle
		if repo != nil {
			if err := repo.UpdateState(context.Background(), inst.ID, domain.StateIdle); err != nil {
				log.Printf("failed to update instance state to idle: %v", err)
			}
		}
	} else {
		inst.State = domain.StateCrashed
		if repo != nil {
			if err := repo.UpdateState(context.Background(), inst.ID, domain.StateCrashed); err != nil {
				log.Printf("failed to update instance state to crashed: %v", err)
			}
		}
		var report *CrashReport
		if supervisor != nil {
			report = supervisor.AnalyzeCrash(exitCode)
		} else {
			report = &CrashReport{
				Category: CrashCategoryUnknown,
				ExitCode: exitCode,
				Summary:  fmt.Sprintf("Process exited with code %d", exitCode),
			}
		}
		if onCrash != nil {
			onCrash(inst.ID, report)
		}
	}
}

func (s *InstanceService) LaunchWithSupervisor(
	ctx context.Context,
	cfg LaunchConfig,
	javaExec string,
) (ports.ProcessHandle, *LogSupervisor, error) {
	if javaExec == "" {
		javaExec = "java"
	}

	args, err := BuildLaunchArguments(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("build launch args: %w", err)
	}

	supervisor := NewLogSupervisor(200)

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	supervisor.AttachPipes(stdoutR, stderrR)

	s.mu.Lock()
	cfg.Instance.State = domain.StateLaunching
	if s.repo != nil {
		if err := s.repo.UpdateState(ctx, cfg.Instance.ID, domain.StateLaunching); err != nil {
			log.Printf("failed to update instance state to launching: %v", err)
		}
	}
	s.mu.Unlock()

	if cfg.GameDir != "" {
		if err := os.MkdirAll(cfg.GameDir, 0755); err != nil {
			log.Printf("failed to create game dir %s: %v", cfg.GameDir, err)
		}
	}

	handle, err := s.proc.StartProcess(
		ctx,
		javaExec,
		args,
		cfg.GameDir,
		os.Environ(),
		stdoutW,
		stderrW,
	)
	if err != nil {
		s.mu.Lock()
		cfg.Instance.State = domain.StateCrashed
		if s.repo != nil {
			if updateErr := s.repo.UpdateState(ctx, cfg.Instance.ID, domain.StateCrashed); updateErr != nil {
				log.Printf("failed to update instance state to crashed: %v", updateErr)
			}
		}
		s.mu.Unlock()
		_ = stdoutW.Close() // errcheck:ok best-effort pipe cleanup on process launch failure
		_ = stderrW.Close() // errcheck:ok best-effort pipe cleanup on process launch failure
		return nil, nil, fmt.Errorf("start game process: %w", err)
	}

	s.mu.Lock()
	cfg.Instance.State = domain.StateRunning
	now := s.clock.Now()
	cfg.Instance.LastPlayedAt = &now
	if s.repo != nil {
		if err := s.repo.Save(ctx, cfg.Instance); err != nil {
			log.Printf("failed to save instance last played time: %v", err)
		}
	}
	s.mu.Unlock()

	return handle, supervisor, nil
}