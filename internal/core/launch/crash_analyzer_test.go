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

func TestCrashAnalyzer_SubscriptionAndNonBlockingDrop(t *testing.T) {
	sup := launch.NewLogSupervisor(100)
	ch, unsub := sup.Subscribe(2)
	defer unsub()

	// Push 10 lines
	for i := 0; i < 10; i++ {
		sup.ProcessLine(strings.Repeat("a", i+1))
	}

	// Should have received at least 2 lines without blocking
	count := 0
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			if len(line) == 0 {
				t.Fatal("expected non-empty line")
			}
			count++
		default:
			goto done
		}
	}
done:
	if count != 2 {
		t.Fatalf("expected exactly 2 buffered lines due to drop policy, got %d", count)
	}

	// Unsubscribe closes channel
	unsub()
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after unsubscribe")
	}
}