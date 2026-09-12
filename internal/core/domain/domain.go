package domain

import (
	"errors"
	"time"
)

var (
	ErrInstanceNotFound          = errors.New("instance not found")
	ErrAccountNotFound           = errors.New("account not found")
	ErrInvalidConfig             = errors.New("invalid configuration")
	ErrOfflineLaunchUnsupported = errors.New("offline mode is unsupported in v0.1.1, Microsoft account is required to launch Minecraft")
	ErrNoActiveAccount           = errors.New("no active account selected: please log in or select an account")
	ErrVersionNotFound           = errors.New("requested Minecraft version not found in manifest")
	ErrDownloadFailed            = errors.New("failed to download required game files")
	ErrChecksumMismatch          = errors.New("file checksum verification failed")
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

// OSRule describes operating system matching constraints.
type OSRule struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Arch    string `json:"arch,omitempty"`
}

// Rule defines an OS or feature constraint on launch arguments or libraries.
type Rule struct {
	Action   string          `json:"action"` // "allow" or "disallow"
	OS       *OSRule         `json:"os,omitempty"`
	Features map[string]bool `json:"features,omitempty"`
}

// LibraryArtifact describes downloadable library files with checksum verification.
type LibraryArtifact struct {
	Path string `json:"path"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// LibraryDownloads describes the artifact and native classifier downloads for a library.
type LibraryDownloads struct {
	Artifact    *LibraryArtifact           `json:"artifact,omitempty"`
	Classifiers map[string]LibraryArtifact `json:"classifiers,omitempty"`
}

// Library represents a Minecraft dependency jar.
type Library struct {
	Name      string            `json:"name"`
	Rules     []Rule            `json:"rules,omitempty"`
	Downloads LibraryDownloads  `json:"downloads,omitempty"`
	Natives   map[string]string `json:"natives,omitempty"`
	ServerReq bool              `json:"serverreq,omitempty"`
}

// AssetIndexInfo describes the asset index metadata.
type AssetIndexInfo struct {
	ID        string `json:"id"`
	SHA1      string `json:"sha1,omitempty"`
	Size      int64  `json:"size,omitempty"`
	TotalSize int64  `json:"totalSize,omitempty"`
	URL       string `json:"url"`
}

// DownloadArtifactInfo represents a general downloadable file descriptor.
type DownloadArtifactInfo struct {
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// VersionJSON represents the Mojang/Fabric/Forge version descriptor.
type VersionJSON struct {
	ID        string `json:"id"`
	MainClass string `json:"mainClass"`
	Arguments *struct {
		Game []any `json:"game"`
		JVM  []any `json:"jvm"`
	} `json:"arguments,omitempty"`
	MinecraftArguments string         `json:"minecraftArguments,omitempty"`
	Libraries          []Library      `json:"libraries"`
	AssetIndex         AssetIndexInfo `json:"assetIndex"`
	Downloads          struct {
		Client *DownloadArtifactInfo `json:"client,omitempty"`
		Server *DownloadArtifactInfo `json:"server,omitempty"`
	} `json:"downloads,omitempty"`
	Type string `json:"type"`
}

// LaunchConfig contains runtime parameters for constructing game command line.
type LaunchConfig struct {
	Instance       *Instance
	Account        *Account
	VersionMeta    *VersionJSON
	GameDir        string
	AssetsDir      string
	LibrariesDir   string
	NativesDir     string
	ClientJarPath  string
	LibraryJarList []string
	ResolutionW    int
	ResolutionH    int
	IsDemo         bool
}

// EvaluateRules checks if a set of rules permits an argument or library for current OS and architecture.
func EvaluateRules(rules []Rule, currentOS, currentArch string, features map[string]bool) bool {
	if len(rules) == 0 {
		return true // default allow
	}

	allowed := false
	for _, r := range rules {
		match := true

		if r.OS != nil {
			if r.OS.Name != "" {
				expectedOS := r.OS.Name
				if expectedOS == "osx" {
					expectedOS = "darwin"
				}
				if expectedOS != currentOS {
					match = false
				}
			}
			if r.OS.Arch != "" && r.OS.Arch != currentArch {
				match = false
			}
		}

		if len(r.Features) > 0 {
			for k, v := range r.Features {
				if features == nil || features[k] != v {
					match = false
					break
				}
			}
		}

		if match {
			if r.Action == "allow" {
				allowed = true
			} else if r.Action == "disallow" {
				allowed = false
			}
		}
	}

	return allowed
}


