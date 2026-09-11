package domain

import (
	"errors"
	"time"
)

var (
	ErrInstanceNotFound = errors.New("instance not found")
	ErrAccountNotFound  = errors.New("account not found")
	ErrInvalidConfig    = errors.New("invalid configuration")
)

type LoaderType string

const (
	LoaderVanilla  LoaderType = "vanilla"
	LoaderFabric   LoaderType = "fabric"
	LoaderQuilt    LoaderType = "quilt"
	LoaderForge    LoaderType = "forge"
	LoaderNeoForge LoaderType = "neoforge"
)

type InstanceState string

const (
	StateIdle       InstanceState = "idle"
	StateDownloading InstanceState = "downloading"
	StateLaunching   InstanceState = "launching"
	StateRunning     InstanceState = "running"
	StateCrashed     InstanceState = "crashed"
)

// Instance represents an isolated Minecraft installation.
type Instance struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	GameVersion  string        `json:"game_version"`
	Loader       LoaderType    `json:"loader"`
	LoaderVer    string        `json:"loader_version,omitempty"`
	IconPath     string        `json:"icon_path,omitempty"`
	JavaPath     string        `json:"java_path,omitempty"`
	MinRAMMB     int           `json:"min_ram_mb"`
	MaxRAMMB     int           `json:"max_ram_mb"`
	JVMArgs      []string      `json:"jvm_args"`
	State        InstanceState `json:"state"`
	LastPlayedAt *time.Time    `json:"last_played_at,omitempty"`
	TotalPlaySec int64         `json:"total_play_seconds"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type AccountType string

const (
	AccountMicrosoft AccountType = "microsoft"
	AccountOffline   AccountType = "offline"
)

// Account represents an authenticated player profile.
type Account struct {
	UUID        string      `json:"uuid"`
	Username    string      `json:"username"`
	Type        AccountType `json:"type"`
	AccessToken string      `json:"-"` // never serialized to disk/IPC directly
	ExpiresAt   time.Time   `json:"expires_at"`
	IsActive    bool        `json:"is_active"`
}

// DownloadProgress represents a granular download event for high-frequency streams.
type DownloadProgress struct {
	TaskID     string  `json:"task_id"`
	FileName   string  `json:"file_name"`
	BytesRead  int64   `json:"bytes_read"`
	TotalBytes int64   `json:"total_bytes"`
	Percentage float64 `json:"percentage"`
	SpeedBPS   int64   `json:"speed_bps"`
}
