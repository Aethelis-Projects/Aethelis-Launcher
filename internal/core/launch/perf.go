package launch

import "fmt"

// PerformancePreset constants for Feature B (v0.7.1).
// Flag set follows the canonical Aikar flags guidance for G1GC
// (source of truth: https://aikar.co/mcflags/). Server-side properties
// (-Dusing.aikars.flags / -Daikars.new.flags) are intentionally omitted:
// they are Paper server markers and useless for a client JVM. The preset
// is G1-only and must never be combined with -XX:+UseZGC (mutually
// incompatible per the same guidance).
const (
	// SuggestedRAMFloorMB is the minimum suggestion for the RAM heuristic.
	SuggestedRAMFloorMB = 1024
	// SuggestedRAMCeilMB caps the heuristic suggestion: Minecraft gains
	// nothing beyond 4 GB of heap for typical (non-huge-modpack) play and
	// oversized heaps only worsen GC pauses.
	SuggestedRAMCeilMB = 4096
	// SuggestedRAMFallbackMB is used when physical memory cannot be detected.
	SuggestedRAMFallbackMB = 2048
	// AikarSmallHeapCutoffMB selects the <12 GB variant of the flag set
	// (G1NewSizePercent=40/G1MaxNewSizePercent=50, region 8M).
	AikarSmallHeapCutoffMB = 12288
)

// SuggestRAMMB converts total physical memory into a recommended heap size:
// min(4096, total/4), floored at 1024. Unknown total (0 or error) yields the
// conservative fallback of 2048 MB.
func SuggestRAMMB(totalPhysicalMB int) int {
	if totalPhysicalMB <= 0 {
		return SuggestedRAMFallbackMB
	}
	suggested := totalPhysicalMB / 4
	if suggested < SuggestedRAMFloorMB {
		return SuggestedRAMFloorMB
	}
	if suggested > SuggestedRAMCeilMB {
		return SuggestedRAMCeilMB
	}
	return suggested
}

// AikarArgs returns the canonical G1GC tuning flags for a heap of heapMB
// (heapMB <= 0 is treated as the 4096 MB default). -Xms/-Xmx are NOT included:
// the launcher composes them from instance Min/MaxRAMMB in BuildArgs, and
// duplicating them here would fight with user settings.
func AikarArgs(heapMB int) []string {
	if heapMB <= 0 {
		heapMB = SuggestedRAMCeilMB
	}
	newSize, maxNewSize, region := 30, 40, "8M"
	if heapMB >= AikarSmallHeapCutoffMB {
		newSize, maxNewSize, region = 40, 50, "16M"
	}
	return []string{
		"-XX:+UseG1GC",
		"-XX:+ParallelRefProcEnabled",
		"-XX:MaxGCPauseMillis=200",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:+DisableExplicitGC",
		"-XX:+AlwaysPreTouch",
		fmt.Sprintf("-XX:G1NewSizePercent=%d", newSize),
		fmt.Sprintf("-XX:G1MaxNewSizePercent=%d", maxNewSize),
		fmt.Sprintf("-XX:G1HeapRegionSize=%s", region),
		"-XX:G1ReservePercent=20",
		"-XX:G1HeapWastePercent=5",
		"-XX:G1MixedGCCountTarget=4",
		"-XX:InitiatingHeapOccupancyPercent=15",
		"-XX:G1MixedGCLiveThresholdPercent=90",
		"-XX:G1RSetUpdatingPauseTimePercent=5",
		"-XX:SurvivorRatio=32",
		"-XX:+PerfDisableSharedMem",
		"-XX:MaxTenuringThreshold=1",
	}
}

// CuratedOptimizationSlugs returns the Modrinth project slugs of the
// optimization mod set for a loader family. The list is deliberately small
// and curated (no "tune everything" spam); empty result for a loader means
// "nothing vetted here", and the UI must say so instead of guessing.
// Known gaps are documented in README ("Optimization set").
func CuratedOptimizationSlugs(loader string) []string {
	switch loader {
	case "fabric":
		return []string{"sodium", "lithium", "ferrite-core", "modernfix"}
	case "quilt":
		return []string{"sodium", "ferrite-core"}
	case "neoforge":
		return []string{"oculus", "ferrite-core", "modernfix"}
	case "forge":
		return []string{"oculus", "ferrite-core", "modernfix"}
	default:
		return nil
	}
}
