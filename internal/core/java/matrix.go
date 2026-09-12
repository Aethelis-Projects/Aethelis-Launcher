package java

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

//go:embed java_matrix.json
var matrixJSON []byte

type Rule struct {
	MinVersion  string `json:"min_version"`
	MaxVersion  string `json:"max_version"`
	JavaMajor   int    `json:"java_major"`
	Description string `json:"description"`
}

type MatrixConfig struct {
	Rules []Rule `json:"rules"`
}

var defaultMatrix MatrixConfig

func init() {
	if err := json.Unmarshal(matrixJSON, &defaultMatrix); err != nil {
		panic(fmt.Sprintf("failed to parse embedded java_matrix.json: %v", err))
	}
}

// parseVersion converts "1.20.5" into []int{1, 20, 5}.
func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	res := make([]int, len(parts))
	for i, p := range parts {
		// Handle non-digit suffixes if any (e.g. "1.20-rc1")
		numStr := ""
		for _, ch := range p {
			if ch >= '0' && ch <= '9' {
				numStr += string(ch)
			} else {
				break
			}
		}
		if numStr == "" {
			numStr = "0"
		}
		n, _ := strconv.Atoi(numStr)
		res[i] = n
	}
	return res
}

// compareVersion returns:
// -1 if a < b
//  0 if a == b
//  1 if a > b
func compareVersion(a, b string) int {
	pa := parseVersion(a)
	pb := parseVersion(b)

	maxLen := len(pa)
	if len(pb) > maxLen {
		maxLen = len(pb)
	}

	for i := 0; i < maxLen; i++ {
		na, nb := 0, 0
		if i < len(pa) {
			na = pa[i]
		}
		if i < len(pb) {
			nb = pb[i]
		}
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
	}
	return 0
}

// ResolveJavaMajor determines the required Java major version (e.g., 8, 17, 21) for a given Minecraft version.
func ResolveJavaMajor(mcVersion string) (int, error) {
	if mcVersion == "" {
		return 21, fmt.Errorf("empty minecraft version provided")
	}

	for _, rule := range defaultMatrix.Rules {
		matchesMin := true
		if rule.MinVersion != "" {
			matchesMin = compareVersion(mcVersion, rule.MinVersion) >= 0
		}

		matchesMax := true
		if rule.MaxVersion != "" {
			matchesMax = compareVersion(mcVersion, rule.MaxVersion) <= 0
		}

		if matchesMin && matchesMax {
			return rule.JavaMajor, nil
		}
	}

	// Default modern fallback
	return 21, nil
}
