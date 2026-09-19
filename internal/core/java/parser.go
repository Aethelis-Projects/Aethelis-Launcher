package java

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Matches lines like:
	// java version "1.8.0_391"
	// openjdk version "21.0.2" 2024-01-16 LTS
	// openjdk version '17.0.1'
	// java version 17.0.2
	javaVersionRegex = regexp.MustCompile(`(?i)(?:java|openjdk)\s+version\s+["']?([0-9]+(?:\.[0-9]+)*[^\s"']*)["']?`)
	// Fallback for generic "version X.Y.Z"
	genericVersionRegex = regexp.MustCompile(`(?i)\bversion\s+["']?([0-9]+(?:\.[0-9]+)*[^\s"']*)["']?`)
)

// ParseJavaMajor parses the Java major version (e.g. 8, 17, 21) from `java -version` command
// stdout/stderr output or raw version strings.
// Handles single-quote, double-quote, unquoted, and vendor prefix variations.
func ParseJavaMajor(output string) (int, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return 0, fmt.Errorf("empty java version output")
	}

	// 1. Scan line by line for version signatures
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if match := javaVersionRegex.FindStringSubmatch(line); len(match) >= 2 {
			if major := extractMajor(match[1]); major > 0 {
				return major, nil
			}
		}

		if match := genericVersionRegex.FindStringSubmatch(line); len(match) >= 2 {
			if major := extractMajor(match[1]); major > 0 {
				return major, nil
			}
		}
	}

	// 2. Direct version string parsing fallback (e.g. "21.0.2", "1.8.0_391", "17")
	clean := strings.Trim(trimmed, `"'`)
	if major := extractMajor(clean); major > 0 {
		return major, nil
	}

	return 0, fmt.Errorf("unable to determine Java major version from output: %q", output)
}

func extractMajor(v string) int {
	clean := strings.Trim(v, `"'`)

	// Java 1.8, 1.7, 1.6 etc.
	if strings.HasPrefix(clean, "1.") {
		parts := strings.Split(clean, ".")
		if len(parts) >= 2 {
			m, _ := strconv.Atoi(digitsOnly(parts[1]))
			return m
		}
	}

	// Java 9+ (e.g. 17.0.2, 21.0.2, 21)
	parts := strings.Split(clean, ".")
	if len(parts) >= 1 {
		m, _ := strconv.Atoi(digitsOnly(parts[0]))
		return m
	}

	return 0
}

func digitsOnly(s string) string {
	var sb strings.Builder
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			sb.WriteRune(ch)
		} else {
			break
		}
	}
	return sb.String()
}
