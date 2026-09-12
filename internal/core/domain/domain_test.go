package domain_test

import (
	"testing"

	"github.com/nord-launcher/launcher/internal/core/domain"
)

func TestEvaluateRules_EmptyRules(t *testing.T) {
	if !domain.EvaluateRules(nil, "windows", "amd64", nil) {
		t.Errorf("expected empty rules to allow by default")
	}
	if !domain.EvaluateRules([]domain.Rule{}, "windows", "amd64", nil) {
		t.Errorf("expected empty slice rules to allow by default")
	}
}

func TestEvaluateRules_OSMatching(t *testing.T) {
	rules := []domain.Rule{
		{
			Action: "allow",
			OS: &domain.OSRule{
				Name: "windows",
			},
		},
	}

	if !domain.EvaluateRules(rules, "windows", "amd64", nil) {
		t.Errorf("expected allow on windows")
	}
	if domain.EvaluateRules(rules, "linux", "amd64", nil) {
		t.Errorf("expected disallow on linux")
	}

	// OSX to Darwin mapping
	osxRules := []domain.Rule{
		{
			Action: "allow",
			OS: &domain.OSRule{
				Name: "osx",
			},
		},
	}
	if !domain.EvaluateRules(osxRules, "darwin", "arm64", nil) {
		t.Errorf("expected osx rule to match darwin")
	}
	if domain.EvaluateRules(osxRules, "windows", "amd64", nil) {
		t.Errorf("expected osx rule to not match windows")
	}
}

func TestEvaluateRules_ArchMatching(t *testing.T) {
	rules := []domain.Rule{
		{
			Action: "allow",
			OS: &domain.OSRule{
				Arch: "x86",
			},
		},
	}

	if !domain.EvaluateRules(rules, "windows", "x86", nil) {
		t.Errorf("expected allow on x86 arch")
	}
	if domain.EvaluateRules(rules, "windows", "amd64", nil) {
		t.Errorf("expected disallow on amd64 arch")
	}
}

func TestEvaluateRules_Features(t *testing.T) {
	rules := []domain.Rule{
		{
			Action: "allow",
			Features: map[string]bool{
				"is_demo_user": true,
			},
		},
	}

	if domain.EvaluateRules(rules, "windows", "amd64", nil) {
		t.Errorf("expected disallow when features map is nil")
	}
	if domain.EvaluateRules(rules, "windows", "amd64", map[string]bool{"is_demo_user": false}) {
		t.Errorf("expected disallow when is_demo_user is false")
	}
	if !domain.EvaluateRules(rules, "windows", "amd64", map[string]bool{"is_demo_user": true}) {
		t.Errorf("expected allow when is_demo_user is true")
	}
}

func TestEvaluateRules_DisallowAction(t *testing.T) {
	rules := []domain.Rule{
		{
			Action: "allow",
		},
		{
			Action: "disallow",
			OS: &domain.OSRule{
				Name: "osx",
			},
		},
	}

	if !domain.EvaluateRules(rules, "windows", "amd64", nil) {
		t.Errorf("expected allow on windows")
	}
	if domain.EvaluateRules(rules, "darwin", "amd64", nil) {
		t.Errorf("expected disallow on darwin/osx")
	}
}
