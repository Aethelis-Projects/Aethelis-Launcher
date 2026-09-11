package launch_test

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/launch"
)

func TestBuildLaunchArguments_Modern(t *testing.T) {
	versionRaw := `{
		"id": "1.21.1",
		"mainClass": "net.minecraft.client.main.Main",
		"arguments": {
			"jvm": [
				"-Djava.library.path=${natives_directory}",
				"-Dminecraft.launcher.brand=${launcher_name}",
				"-Dminecraft.launcher.version=${launcher_version}",
				"-cp",
				"${classpath}",
				{
					"rules": [
						{"action": "allow", "os": {"name": "windows"}}
					],
					"value": "-XX:HeapDumpPath=MojangTricksIntelDriversForPerformance_file.r51"
				}
			],
			"game": [
				"--username", "${auth_player_name}",
				"--version", "${version_name}",
				"--gameDir", "${game_directory}",
				"--assetsDir", "${assets_root}",
				"--assetIndex", "${assets_index_name}",
				"--uuid", "${auth_uuid}",
				"--accessToken", "${auth_access_token}",
				"--userType", "${user_type}",
				"--versionType", "${version_type}",
				"--width", "${resolution_width}",
				"--height", "${resolution_height}"
			]
		},
		"assetIndex": {
			"id": "17"
		}
	}`

	var vMeta launch.VersionJSON
	if err := json.Unmarshal([]byte(versionRaw), &vMeta); err != nil {
		t.Fatalf("unmarshal version json: %v", err)
	}

	inst := &domain.Instance{
		ID:          "nord-test",
		Name:        "NordPack",
		GameVersion: "1.21.1",
		MinRAMMB:    3072,
		MaxRAMMB:    6144,
		JVMArgs:     []string{"-XX:+UseG1GC", "-Duser.language=ru"},
	}

	acc := &domain.Account{
		UUID:        "12345678-abcd-1234-abcd-1234567890ab",
		Username:    "NordHero",
		Type:        domain.AccountMicrosoft,
		AccessToken: "jwt-mc-token-xyz",
	}

	cfg := launch.LaunchConfig{
		Instance:       inst,
		Account:        acc,
		VersionMeta:    &vMeta,
		GameDir:        "C:/Nord/instances/test",
		AssetsDir:      "C:/Nord/assets",
		NativesDir:     "C:/Nord/natives",
		ClientJarPath:  "C:/Nord/versions/1.21.1/client.jar",
		LibraryJarList: []string{"C:/Nord/libraries/lib1.jar", "C:/Nord/libraries/lib2.jar"},
		ResolutionW:    1920,
		ResolutionH:    1080,
	}

	args, err := launch.BuildLaunchArguments(cfg)
	if err != nil {
		t.Fatalf("build launch arguments failed: %v", err)
	}

	cmdLine := strings.Join(args, " ")

	// 1. RAM flags
	if !strings.Contains(cmdLine, "-Xms3072M") || !strings.Contains(cmdLine, "-Xmx6144M") {
		t.Errorf("missing RAM args in cmdline: %s", cmdLine)
	}

	// 2. Custom JVM args
	if !strings.Contains(cmdLine, "-XX:+UseG1GC") || !strings.Contains(cmdLine, "-Duser.language=ru") {
		t.Errorf("missing custom JVM args: %s", cmdLine)
	}

	// 3. Classpath with separator
	sep := string(os.PathListSeparator)
	expectedCP := "C:/Nord/libraries/lib1.jar" + sep + "C:/Nord/libraries/lib2.jar" + sep + "C:/Nord/versions/1.21.1/client.jar"
	if !strings.Contains(cmdLine, expectedCP) {
		t.Errorf("expected classpath %q, got: %s", expectedCP, cmdLine)
	}

	// 4. Main Class
	if !strings.Contains(cmdLine, "net.minecraft.client.main.Main") {
		t.Errorf("missing main class: %s", cmdLine)
	}

	// 5. Game variables
	if !strings.Contains(cmdLine, "--username NordHero") {
		t.Errorf("missing username: %s", cmdLine)
	}
	if !strings.Contains(cmdLine, "--uuid 12345678-abcd-1234-abcd-1234567890ab") {
		t.Errorf("missing uuid: %s", cmdLine)
	}
	if !strings.Contains(cmdLine, "--accessToken jwt-mc-token-xyz") {
		t.Errorf("missing access token: %s", cmdLine)
	}
	if !strings.Contains(cmdLine, "--width 1920 --height 1080") {
		t.Errorf("missing resolution: %s", cmdLine)
	}

	// 6. OS Rules
	if runtime.GOOS == "windows" {
		if !strings.Contains(cmdLine, "MojangTricksIntelDriversForPerformance") {
			t.Errorf("expected Windows-specific rule to match on Windows: %s", cmdLine)
		}
	}
}

func TestBuildLaunchArguments_Legacy(t *testing.T) {
	versionRaw := `{
		"id": "1.12.2",
		"mainClass": "net.minecraft.client.main.Main",
		"minecraftArguments": "--username ${auth_player_name} --version ${version_name} --gameDir ${game_directory} --assetsDir ${assets_root} --assetIndex ${assets_index_name} --uuid ${auth_uuid} --accessToken ${auth_access_token} --userType ${user_type} --versionType ${version_type}"
	}`

	var vMeta launch.VersionJSON
	_ = json.Unmarshal([]byte(versionRaw), &vMeta)

	inst := &domain.Instance{
		ID:          "legacy-test",
		Name:        "Legacy112",
		GameVersion: "1.12.2",
	}

	acc := &domain.Account{
		UUID:        "offline-uuid",
		Username:    "LegacyPlayer",
		Type:        domain.AccountOffline,
		AccessToken: "0",
	}

	cfg := launch.LaunchConfig{
		Instance:      inst,
		Account:       acc,
		VersionMeta:   &vMeta,
		GameDir:       "C:/Nord/instances/legacy",
		ClientJarPath: "C:/Nord/client.jar",
	}

	args, err := launch.BuildLaunchArguments(cfg)
	if err != nil {
		t.Fatalf("build legacy arguments failed: %v", err)
	}

	cmdLine := strings.Join(args, " ")
	if !strings.Contains(cmdLine, "--username LegacyPlayer") {
		t.Errorf("expected legacy player name in args: %s", cmdLine)
	}
	if !strings.Contains(cmdLine, "-cp C:/Nord/client.jar") {
		t.Errorf("expected classpath in default args: %s", cmdLine)
	}
}