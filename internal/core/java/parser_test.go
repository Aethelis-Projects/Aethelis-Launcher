package java_test

import (
	"testing"

	"github.com/nord-launcher/launcher/internal/core/java"
)

func TestParseJavaMajor(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		expectedMajor int
		expectErr     bool
	}{
		{
			name: "Oracle Java 8 double quotes",
			output: `java version "1.8.0_391"
Java(TM) SE Runtime Environment (build 1.8.0_391-b13)
Java HotSpot(TM) 64-Bit Server VM (build 25.391-b13, mixed mode)`,
			expectedMajor: 8,
			expectErr:     false,
		},
		{
			name: "OpenJDK 21 double quotes with LTS",
			output: `openjdk version "21.0.2" 2024-01-16 LTS
OpenJDK Runtime Environment Temurin-21.0.2+13 (build 21.0.2+13)
OpenJDK 64-Bit Server VM Temurin-21.0.2+13 (build 21.0.2+13, mixed mode, sharing)`,
			expectedMajor: 21,
			expectErr:     false,
		},
		{
			name: "OpenJDK 17 single quotes",
			output: `openjdk version '17.0.10' 2024-01-16
OpenJDK Runtime Environment (build 17.0.10+7)
OpenJDK 64-Bit Server VM (build 17.0.10+7, mixed mode, sharing)`,
			expectedMajor: 17,
			expectErr:     false,
		},
		{
			name: "Unquoted java version 17.0.2",
			output: `java version 17.0.2 2022-01-18
Java(TM) SE Runtime Environment (build 17.0.2+8-86)
Java HotSpot(TM) 64-Bit Server VM (build 17.0.2+8-86, mixed mode, sharing)`,
			expectedMajor: 17,
			expectErr:     false,
		},
		{
			name: "Unquoted openjdk version 21",
			output: `openjdk version 21 2023-09-19
OpenJDK Runtime Environment (build 21+35-2513)`,
			expectedMajor: 21,
			expectErr:     false,
		},
		{
			name:          "Direct version string 21.0.2",
			output:        "21.0.2",
			expectedMajor: 21,
			expectErr:     false,
		},
		{
			name:          "Direct version string 1.8.0_401",
			output:        "1.8.0_401",
			expectedMajor: 8,
			expectErr:     false,
		},
		{
			name:          "Direct version string with quotes",
			output:        `"17.0.9"`,
			expectedMajor: 17,
			expectErr:     false,
		},
		{
			name:          "Empty output",
			output:        "",
			expectedMajor: 0,
			expectErr:     true,
		},
		{
			name:          "Invalid non-java output",
			output:        "bash: java: command not found",
			expectedMajor: 0,
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, err := java.ParseJavaMajor(tt.output)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for output %q, got nil (major: %d)", tt.output, major)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for output %q: %v", tt.output, err)
			}

			if major != tt.expectedMajor {
				t.Errorf("expected major %d, got %d for output %q", tt.expectedMajor, major, tt.output)
			}
		})
	}
}
