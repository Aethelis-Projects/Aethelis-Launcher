package launch_test

import (
	"strings"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/launch"
)

func TestCrashAnalyzer_OutOfMemory(t *testing.T) {
	sup := launch.NewLogSupervisor(50)
	sup.ProcessLine("[12:00:00] [main/INFO]: Loading Minecraft 1.21.1...")
	sup.ProcessLine("[12:00:05] [main/ERROR]: java.lang.OutOfMemoryError: Java heap space")
	sup.ProcessLine("[12:00:05] [main/ERROR]: Failed to allocate 1048576 bytes")

	report := sup.AnalyzeCrash(1)
	if report == nil {
		t.Fatal("expected non-nil crash report")
	}

	if report.Category != launch.CrashCategoryOOM {
		t.Fatalf("expected category OOM, got %s", report.Category)
	}
	if !strings.Contains(report.Remedy, "RAM") {
		t.Fatalf("expected RAM remedy, got %s", report.Remedy)
	}
}

func TestCrashAnalyzer_GraphicsDriver(t *testing.T) {
	sup := launch.NewLogSupervisor(50)
	sup.ProcessLine("[12:00:00] [main/INFO]: Setting up window...")
	sup.ProcessLine("[12:00:01] [main/FATAL]: GLFW error 65542: WGL: The driver does not appear to support OpenGL")

	report := sup.AnalyzeCrash(65542)
	if report == nil {
		t.Fatal("expected report")
	}

	if report.Category != launch.CrashCategoryGraphics {
		t.Fatalf("expected category Graphics, got %s", report.Category)
	}
	if !strings.Contains(report.Remedy, "видеокарты") {
		t.Fatalf("expected GPU driver recommendation: %s", report.Remedy)
	}
}

func TestCrashAnalyzer_FabricModConflict(t *testing.T) {
	sup := launch.NewLogSupervisor(50)
	sup.ProcessLine("[12:00:00] [main/INFO]: Loading Fabric...")
	sup.ProcessLine("net.fabricmc.loader.impl.FormattedException: Mod 'sodium' (0.5.8) requires 'fabric-api' but none was found!")

	report := sup.AnalyzeCrash(1)
	if report == nil {
		t.Fatal("expected report")
	}

	if report.Category != launch.CrashCategoryModConflict {
		t.Fatalf("expected category ModConflict, got %s", report.Category)
	}
	if !strings.Contains(report.Summary, "sodium") {
		t.Fatalf("expected summary to identify offending mod sodium: %s", report.Summary)
	}
}

func TestCrashAnalyzer_JavaVersionMismatch(t *testing.T) {
	sup := launch.NewLogSupervisor(50)
	sup.ProcessLine("Exception in thread \"main\" java.lang.UnsupportedClassVersionError: net/minecraft/client/main/Main has been compiled by a more recent version of the Java Runtime (class file version 65.0), this version of the Java Runtime only recognizes class file versions up to 61.0")

	report := sup.AnalyzeCrash(1)
	if report == nil {
		t.Fatal("expected report")
	}

	if report.Category != launch.CrashCategoryJavaVersion {
		t.Fatalf("expected category JavaVersion, got %s", report.Category)
	}
	if !strings.Contains(report.Remedy, "Java 21") {
		t.Fatalf("expected Java 21 mention, got %s", report.Remedy)
	}
}

func TestCrashAnalyzer_CleanExit(t *testing.T) {
	sup := launch.NewLogSupervisor(50)
	sup.ProcessLine("[12:00:00] [main/INFO]: Stopping!")
	sup.ProcessLine("[12:00:01] [main/INFO]: Saving worlds...")

	report := sup.AnalyzeCrash(0)
	if report != nil {
		t.Fatalf("expected nil report on clean exit, got %+v", report)
	}
}

func TestCrashAnalyzer_DefaultSizeAndUnknown(t *testing.T) {
	sup := launch.NewLogSupervisor(0)
	sup.ProcessLine("[12:00:00] [main/INFO]: Random unknown output")
	report := sup.AnalyzeCrash(1)
	if report == nil || report.Category != launch.CrashCategoryUnknown {
		t.Fatalf("expected CrashCategoryUnknown, got: %+v", report)
	}
}