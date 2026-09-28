//go:build ignore

// release_metrics.go compiles metrics.json - the CI provenance for the
// README performance table (owner directive v0.7.2: numbers must originate
// from a release build in CI, never from an ad-hoc sandbox run).
//
//	go run ./scripts/release_metrics.go --version v0.7.2 --tar-bytes 17000000 ... --out release-assets/metrics.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
)

type metrics struct {
	Version               string `json:"version"`
	Source                string `json:"source"`
	LinuxTarballBytes     int64  `json:"linux_tarball_bytes"`
	LinuxBinaryBytes      int64  `json:"linux_binary_bytes"`
	WindowsInstallerBytes int64  `json:"windows_installer_bytes"`
	FrontendBundleGzipKB  int    `json:"frontend_bundle_gzip_kb"`
	CoreCoveragePercent   string `json:"core_coverage_percent"`
	Notes                 string `json:"notes"`
}

func main() {
	m := metrics{Source: "github.com/nord-launcher/launcher Production Release workflow"}
	v := flag.String("version", "", "release tag")
	tar := flag.String("tar-bytes", "0", "")
	lin := flag.String("linux-binary-bytes", "0", "")
	win := flag.String("windows-installer-bytes", "0", "")
	bundle := flag.String("bundle-gzip-kb", "0", "")
	cov := flag.String("core-coverage", "n/a", "")
	out := flag.String("out", "", "output path")
	flag.Parse()

	i64 := func(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
	i := func(s string) int { n, _ := strconv.Atoi(s); return n }
	m.Version = *v
	m.LinuxTarballBytes = i64(*tar)
	m.LinuxBinaryBytes = i64(*lin)
	m.WindowsInstallerBytes = i64(*win)
	m.FrontendBundleGzipKB = i(*bundle)
	m.CoreCoveragePercent = *cov
	m.Notes = "IPC latency deliberately not published; nanoseconds per dispatch are an internal benchmark, not a user-facing metric."

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}
	if *out == "" {
		fmt.Println(string(b))
		return
	}
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Println("metrics.json written to", *out)
}
