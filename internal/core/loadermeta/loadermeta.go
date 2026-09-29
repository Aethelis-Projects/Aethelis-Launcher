// Package loadermeta resolves mod-loader release channels for the instance
// creation wizard (v0.7.2 G10). All sources are official public metadata
// services; failures degrade to an empty option list with the error kept for
// honest UI display (the wizard can still proceed with an empty/typed loader
// version where the loader accepts it).
package loadermeta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	FabricMetaURL  = "https://meta.fabricmc.net/v2/versions/loader/"
	QuiltMetaURL   = "https://meta.quiltmc.org/v3/versions/loader/"
	ForgePromoURL  = "https://files.minecraftforge.net/maven/net/minecraftforge/forge/promotions_slim.json"
	NeoForgeTagsRU = "https://api.github.com/repos/neoforged/NeoForge/tags?per_page=100"
)

// Options is the resolution result for one (game version, loader) pair.
type Options struct {
	Loader  string   `json:"loader"`
	GameVer string   `json:"game_version"`
	Default string   `json:"default"` // recommended / latest-stable pick
	Options []string `json:"options"` // newest first, capped
	Source  string   `json:"source"`  // fabric|quilt|forge|neoforge|vanilla
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Resolver fetches loader metadata with a shared client and short timeouts.
type Resolver struct {
	client    httpClient
	FabricURL string
	QuiltURL  string
	ForgeURL  string
	NeoURL    string
}

// NewResolver builds a resolver over the provided client (nil-safe default).
func NewResolver(c httpClient) *Resolver {
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	return &Resolver{
		client:    c,
		FabricURL: FabricMetaURL,
		QuiltURL:  QuiltMetaURL,
		ForgeURL:  ForgePromoURL,
		NeoURL:    NeoForgeTagsRU,
	}
}

func (r *Resolver) getJSON(ctx context.Context, url string, out any) error {
	if r == nil || r.client == nil || url == "" {
		return errors.New("loader metadata resolver not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Nord-Launcher")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("loader meta HTTP %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// Resolve returns loader version options for (gameVersion, loader).
// loader "vanilla" yields an empty authoritative result.
func (r *Resolver) Resolve(ctx context.Context, loader, gameVersion string) (Options, error) {
	switch strings.ToLower(strings.TrimSpace(loader)) {
	case "", "vanilla":
		return Options{Loader: "vanilla", GameVer: gameVersion, Source: "vanilla"}, nil
	case "fabric":
		return r.resolveFabric(ctx, gameVersion)
	case "quilt":
		return r.resolveQuilt(ctx, gameVersion)
	case "forge":
		return r.resolveForge(ctx, gameVersion)
	case "neoforge":
		return r.resolveNeoForge(ctx, gameVersion)
	default:
		return Options{}, fmt.Errorf("unsupported loader %q", loader)
	}
}

type fabricEntry struct {
	Loader struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	} `json:"loader"`
}

func (r *Resolver) resolveFabric(ctx context.Context, gv string) (Options, error) {
	opt := Options{Loader: "fabric", GameVer: gv, Source: "fabric"}
	var entries []fabricEntry
	if err := r.getJSON(ctx, r.FabricURL+gameVersionSegment(gv), &entries); err != nil {
		return opt, err
	}
	for _, e := range entries {
		if e.Loader.Version == "" {
			continue
		}
		opt.Options = append(opt.Options, e.Loader.Version)
		if e.Loader.Stable && opt.Default == "" {
			opt.Default = e.Loader.Version
		}
	}
	opt.Options = capList(opt.Options)
	if opt.Default == "" && len(opt.Options) > 0 {
		opt.Default = opt.Options[0]
	}
	return opt, nil
}

type quiltEntry struct {
	Loader struct {
		Version string `json:"version"`
	} `json:"loader"`
}

func (r *Resolver) resolveQuilt(ctx context.Context, gv string) (Options, error) {
	opt := Options{Loader: "quilt", GameVer: gv, Source: "quilt"}
	var entries []quiltEntry
	if err := r.getJSON(ctx, r.QuiltURL+gameVersionSegment(gv), &entries); err != nil {
		return opt, err
	}
	for _, e := range entries {
		if e.Loader.Version != "" {
			opt.Options = append(opt.Options, e.Loader.Version)
		}
	}
	opt.Options = capList(opt.Options)
	if len(opt.Options) > 0 {
		opt.Default = opt.Options[0] // meta is newest-first
	}
	return opt, nil
}

func (r *Resolver) resolveForge(ctx context.Context, gv string) (Options, error) {
	opt := Options{Loader: "forge", GameVer: gv, Source: "forge"}
	var promo struct {
		Promos map[string]string `json:"promos"`
	}
	if err := r.getJSON(ctx, r.ForgeURL, &promo); err != nil {
		return opt, err
	}
	// Forge publishes forge-<build> for "<mc>-recommended" / "<mc>-latest";
	// the full maven version keeps the "-forge" suffix visible to users? No:
	// instance provisioning expects the raw build string (e.g. "47.3.0").
	if v, ok := promo.Promos[gv+"-recommended"]; ok && v != "" {
		opt.Default = strings.TrimSuffix(v, "-forge")
	} else if v, ok := promo.Promos[gv+"-latest"]; ok && v != "" {
		opt.Default = strings.TrimSuffix(v, "-forge")
	}
	opt.Options = capList([]string{opt.Default})
	return opt, nil
}

var neoTagRe = regexp.MustCompile(`^(\d+)\.(\d+)`)

func (r *Resolver) resolveNeoForge(ctx context.Context, gv string) (Options, error) {
	opt := Options{Loader: "neoforge", GameVer: gv, Source: "neoforge"}
	var tags []struct {
		Name string `json:"name"`
	}
	if err := r.getJSON(ctx, r.NeoURL, &tags); err != nil {
		return opt, err
	}
	wantMajor, wantMinor, ok := neoforgeLine(gv)
	if !ok {
		// NeoForge starts at the MC 1.21 line; older versions have none.
		return opt, nil
	}
	for _, t := range tags {
		name := strings.TrimPrefix(strings.TrimSpace(t.Name), "v")
		m := neoTagRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		// Match the exact game line (21.4 for 1.21.4); for the "latest"
		// line without a minor (1.21) accept any 21.x tag.
		if major != wantMajor || (wantMinor >= 0 && minor != wantMinor) {
			continue
		}
		opt.Options = append(opt.Options, name)
	}
	opt.Options = capList(opt.Options)
	if len(opt.Options) > 0 {
		opt.Default = opt.Options[0] // GitHub tags are newest-first
	}
	return opt, nil
}

// neoforgeLine maps a Minecraft version to the NeoForge major.minor line.
// The numeric core of the MC version carries over: 1.21.4 -> 21.4,
// 26.1.x -> 26.x (year-line ids use the first component directly).
func neoforgeLine(gv string) (int, int, bool) {
	core := strings.TrimPrefix(gv, "1.")
	parts := strings.Split(core, ".")
	if len(parts) < 2 {
		parts = append(parts, "0")
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 21 {
		return 0, 0, false
	}
	if parts[1] == "" {
		return major, -1, true // bare "1.21" line: any minor
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func gameVersionSegment(gv string) string {
	return strings.TrimSpace(gv)
}

func capList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
		if len(out) >= 30 {
			break
		}
	}
	return out
}
