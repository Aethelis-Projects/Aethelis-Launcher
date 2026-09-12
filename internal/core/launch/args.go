package launch

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/nord-launcher/launcher/internal/core/domain"
)

type VersionJSON = domain.VersionJSON
type Library = domain.Library
type Rule = domain.Rule
type LaunchConfig = domain.LaunchConfig


// EvaluateRules checks if a set of rules permits an argument or library for current OS and architecture.
func EvaluateRules(rules []Rule, currentOS, currentArch string, features map[string]bool) bool {
	return domain.EvaluateRules(rules, currentOS, currentArch, features)
}


// BuildLaunchArguments constructs the complete JVM and game arguments list.
func BuildLaunchArguments(cfg LaunchConfig) ([]string, error) {
	if cfg.Instance == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}
	if cfg.Account == nil {
		return nil, fmt.Errorf("account cannot be nil")
	}
	if cfg.VersionMeta == nil {
		return nil, fmt.Errorf("version metadata cannot be nil")
	}

	currentOS := runtime.GOOS
	currentArch := runtime.GOARCH

	// 1. Build Classpath
	cpList := make([]string, 0, len(cfg.LibraryJarList)+1)
	cpList = append(cpList, cfg.LibraryJarList...)
	if cfg.ClientJarPath != "" {
		cpList = append(cpList, cfg.ClientJarPath)
	}
	classpath := strings.Join(cpList, string(os.PathListSeparator))

	// 2. Template variables
	substitutions := map[string]string{
		"${auth_player_name}":  cfg.Account.Username,
		"${version_name}":      cfg.Instance.GameVersion,
		"${game_directory}":    cfg.GameDir,
		"${assets_root}":       cfg.AssetsDir,
		"${assets_index_name}": cfg.VersionMeta.AssetIndex.ID,
		"${auth_uuid}":         cfg.Account.UUID,
		"${auth_access_token}": cfg.Account.AccessToken,
		"${user_type}":         string(cfg.Account.Type),
		"${version_type}":      "Nord-Launcher",
		"${natives_directory}": cfg.NativesDir,
		"${launcher_name}":     "Nord-Launcher",
		"${launcher_version}":  "0.1.0",
		"${classpath}":         classpath,
		"${resolution_width}":  strconv.Itoa(cfg.ResolutionW),
		"${resolution_height}": strconv.Itoa(cfg.ResolutionH),
	}
	if cfg.ResolutionW <= 0 {
		substitutions["${resolution_width}"] = "854"
	}
	if cfg.ResolutionH <= 0 {
		substitutions["${resolution_height}"] = "480"
	}

	substitute := func(s string) string {
		for k, v := range substitutions {
			s = strings.ReplaceAll(s, k, v)
		}
		return s
	}

	var args []string

	// 3. JVM RAM arguments
	minRAM := cfg.Instance.MinRAMMB
	if minRAM <= 0 {
		minRAM = 2048
	}
	maxRAM := cfg.Instance.MaxRAMMB
	if maxRAM <= 0 {
		maxRAM = 4096
	}
	args = append(args, fmt.Sprintf("-Xms%dM", minRAM))
	args = append(args, fmt.Sprintf("-Xmx%dM", maxRAM))

	// 4. Custom instance JVM arguments
	for _, userArg := range cfg.Instance.JVMArgs {
		if strings.TrimSpace(userArg) != "" {
			args = append(args, userArg)
		}
	}

	// 5. Version-defined JVM arguments
	features := map[string]bool{
		"is_demo_user": cfg.IsDemo,
	}

	hasClasspathArg := false
	if cfg.VersionMeta.Arguments != nil && len(cfg.VersionMeta.Arguments.JVM) > 0 {
		for _, item := range cfg.VersionMeta.Arguments.JVM {
			switch val := item.(type) {
			case string:
				substituted := substitute(val)
				if strings.Contains(val, "${classpath}") {
					hasClasspathArg = true
				}
				args = append(args, substituted)
			case map[string]any:
				// Rule-conditional argument
				var rules []Rule
				if rBytes, err := json.Marshal(val["rules"]); err == nil {
					_ = json.Unmarshal(rBytes, &rules) // slop:ok best-effort unmarshal of rule objects
				}
				if EvaluateRules(rules, currentOS, currentArch, features) {
					switch value := val["value"].(type) {
					case string:
						args = append(args, substitute(value))
					case []any:
						for _, sub := range value {
							if s, ok := sub.(string); ok {
								args = append(args, substitute(s))
							}
						}
					}
				}
			}
		}
	} else {
		// Default standard JVM arguments if not specified
		if cfg.NativesDir != "" {
			args = append(args, fmt.Sprintf("-Djava.library.path=%s", cfg.NativesDir))
		}
		args = append(args, "-Dminecraft.launcher.brand=Nord-Launcher")
		args = append(args, "-Dminecraft.launcher.version=0.1.0")
	}

	if !hasClasspathArg {
		args = append(args, "-cp", classpath)
	}

	// 6. Main Class
	mainClass := cfg.VersionMeta.MainClass
	if mainClass == "" {
		mainClass = "net.minecraft.client.main.Main"
	}
	args = append(args, mainClass)

	// 7. Game arguments
	if cfg.VersionMeta.Arguments != nil && len(cfg.VersionMeta.Arguments.Game) > 0 {
		for _, item := range cfg.VersionMeta.Arguments.Game {
			switch val := item.(type) {
			case string:
				args = append(args, substitute(val))
			case map[string]any:
				var rules []Rule
				if rBytes, err := json.Marshal(val["rules"]); err == nil {
					_ = json.Unmarshal(rBytes, &rules) // slop:ok best-effort unmarshal of rule objects
				}
				if EvaluateRules(rules, currentOS, currentArch, features) {
					switch value := val["value"].(type) {
					case string:
						args = append(args, substitute(value))
					case []any:
						for _, sub := range value {
							if s, ok := sub.(string); ok {
								args = append(args, substitute(s))
							}
						}
					}
				}
			}
		}
	} else if cfg.VersionMeta.MinecraftArguments != "" {
		// Legacy minecraftArguments string (1.12.2 and older)
		for _, token := range strings.Fields(cfg.VersionMeta.MinecraftArguments) {
			args = append(args, substitute(token))
		}
	}

	return args, nil
}