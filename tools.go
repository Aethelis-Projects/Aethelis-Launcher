//go:build tools
// +build tools

package tools

import (
	_ "github.com/google/go-licenses"
	_ "golang.org/x/vuln/cmd/govulncheck"
)
