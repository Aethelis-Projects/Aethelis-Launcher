package launch

import (
	"reflect"
	"strings"
	"testing"
)

func TestSuggestRAMMB(t *testing.T) {
	cases := []struct {
		total int
		want  int
	}{
		{-4096, SuggestedRAMFallbackMB},
		{0, SuggestedRAMFallbackMB},
		{2048, SuggestedRAMFloorMB},
		{4096, 1024},
		{8192, 2048},
		{16384, 4096},
		{65536, SuggestedRAMCeilMB},
	}
	for _, c := range cases {
		if got := SuggestRAMMB(c.total); got != c.want {
			t.Errorf("SuggestRAMMB(%d) = %d, want %d", c.total, got, c.want)
		}
	}
}

func TestAikarArgsSmallHeap(t *testing.T) {
	args := AikarArgs(4096)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-XX:+UseG1GC",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:G1NewSizePercent=30",
		"-XX:G1MaxNewSizePercent=40",
		"-XX:G1HeapRegionSize=8M",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("small-heap preset missing %q", want)
		}
	}
	// -Xmx/-Xms are owned by BuildArgs; duplicating them here breaks user RAM settings.
	if strings.Contains(joined, "-Xm") {
		t.Error("preset must not contain -Xms/-Xmx")
	}
	// Server-side Aikar markers are useless for a client JVM.
	if strings.Contains(joined, "aikars.flags") || strings.Contains(joined, "aikar") {
		t.Error("preset must not contain server-side aikar markers")
	}
}

func TestAikarArgsLargeHeap(t *testing.T) {
	joined := strings.Join(AikarArgs(16384), " ")
	for _, want := range []string{
		"-XX:G1NewSizePercent=40",
		"-XX:G1MaxNewSizePercent=50",
		"-XX:G1HeapRegionSize=16M",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("large-heap preset missing %q", want)
		}
	}
	if strings.Contains(joined, "ZGC") {
		t.Error("G1 preset must never mix ZGC flags")
	}
}

func TestAikarArgsUnlockPrecedesExperimental(t *testing.T) {
	args := AikarArgs(0)
	unlock, sizing := -1, -1
	for i, a := range args {
		if a == "-XX:+UnlockExperimentalVMOptions" {
			unlock = i
		}
		if strings.HasPrefix(a, "-XX:G1NewSizePercent") {
			sizing = i
		}
	}
	if unlock < 0 || sizing < 0 || unlock > sizing {
		t.Fatalf("UnlockExperimentalVMOptions (%d) must come before experimental sizing (%d)", unlock, sizing)
	}
}

func TestCuratedOptimizationSlugs(t *testing.T) {
	if got := CuratedOptimizationSlugs("fabric"); !reflect.DeepEqual(got, []string{"sodium", "lithium", "ferrite-core", "modernfix"}) {
		t.Errorf("fabric set changed: %v", got)
	}
	if got := CuratedOptimizationSlugs("quilt"); len(got) == 0 {
		t.Error("quilt set must not be empty")
	}
	for _, loader := range []string{"forge", "neoforge"} {
		if got := CuratedOptimizationSlugs(loader); len(got) == 0 {
			t.Errorf("%s set must not be empty", loader)
		}
	}
	for _, loader := range []string{"vanilla", "", "bogus"} {
		if got := CuratedOptimizationSlugs(loader); got != nil {
			t.Errorf("loader %q must have no curated set, got %v", loader, got)
		}
	}
}

func TestPhysicalMemoryMBPlausible(t *testing.T) {
	mb, err := PhysicalMemoryMB()
	if err != nil {
		t.Skipf("platform without physical memory detection: %v", err)
	}
	if mb <= 0 {
		t.Fatalf("expected positive physical memory, got %d MB", mb)
	}
}
