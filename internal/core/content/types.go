package content

import "time"

type ModSource string

const (
	SourceModrinth   ModSource = "modrinth"
	SourceCurseForge ModSource = "curseforge"
)

type DependencyType string

const (
	DepRequired     DependencyType = "required"
	DepOptional     DependencyType = "optional"
	DepIncompatible DependencyType = "incompatible"
	DepEmbedded     DependencyType = "embedded"
)

// ModItem is the unified metadata model across Modrinth and CurseForge.
type ModItem struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Source      ModSource `json:"source"`
	Name        string    `json:"name"`
	Author      string    `json:"author"`
	Summary     string    `json:"summary"`
	Description string    `json:"description,omitempty"`
	IconURL     string    `json:"icon_url,omitempty"`
	Downloads   int64     `json:"downloads"`
	Follows     int64     `json:"follows"`
	Categories  []string  `json:"categories"`
	Loaders     []string  `json:"loaders"`
	GameVers    []string  `json:"game_versions"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ModDependency represents a dependency relationship to another mod.
type ModDependency struct {
	ProjectID string         `json:"project_id"`
	VersionID string         `json:"version_id,omitempty"`
	Type      DependencyType `json:"type"`
	FileName  string         `json:"file_name,omitempty"`
}

// ModFile represents a downloadable JAR file.
type ModFile struct {
	ID           string          `json:"id"`
	VersionID    string          `json:"version_id"`
	FileName     string          `json:"file_name"`
	URL          string          `json:"url"`
	Size         int64           `json:"size"`
	SHA1         string          `json:"sha1,omitempty"`
	SHA512       string          `json:"sha512,omitempty"`
	Dependencies []ModDependency `json:"dependencies"`
	Primary      bool            `json:"primary"`
}

// ModVersion represents a specific release of a mod.
type ModVersion struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"project_id"`
	VersionNum   string          `json:"version_number"`
	Name         string          `json:"name"`
	GameVersions []string        `json:"game_versions"`
	Loaders      []string        `json:"loaders"`
	Files        []ModFile       `json:"files"`
	Dependencies []ModDependency `json:"dependencies"`
	ReleaseDate  time.Time       `json:"release_date"`
}

// LoaderVersion represents a game loader version metadata entry.
type LoaderVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Maven   string `json:"maven,omitempty"`
}