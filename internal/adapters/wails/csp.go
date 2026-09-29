package wails

import (
	"net/http"
	"strings"
)

// CSPMiddleware pins the webview to the app's own assets plus the exact
// remote image hosts the UI actually renders (Modrinth/CurseForge CDN icons -
// user data; the round-4 review: default-src 'self' alone would blank every
// mod icon). Scripts stay strict: wails beta.20 runtime.js and the built
// frontend contain no inline script (the dev-only new Function bootstrap lives
// in the Vite source index.html while wails serves the built dist/; CI asserts
// the built index.html has no inline script either).
func CSPMiddleware(next http.Handler) http.Handler {
	policy := CSPPolicy()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", policy)
		next.ServeHTTP(w, r)
	})
}

// CSPPolicy is the full header policy (single source of truth).
func CSPPolicy() string {
	return "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob: https://cdn.modrinth.com https://*.modrinth.com https://mediafiles.forgecdn.net https://*.forgecdn.net; " +
		"font-src 'self' data:; " +
		"connect-src 'self'; " +
		"object-src 'none'; " +
		"base-uri 'none'; " +
		"form-action 'none'; " +
		"frame-ancestors 'none'"
}

// MetaOnlyDirectives are ignored when delivered via <meta http-equiv> per the
// CSP spec, and browsers log "Ignored via <meta>: 'frame-ancestors'" - which
// would break the "clean console" acceptance item. They are therefore header-
// only, and the meta twin uses CSPPolicyForMeta().
func metaUnsafeDirectives() []string {
	return []string{"frame-ancestors", "sandbox", "report-uri", "report-to", "require-trusted-types-for", "trusted-types"}
}

// CSPPolicyForMeta is CSPPolicy() minus the directives a <meta> cannot carry.
func CSPPolicyForMeta() string {
	keep := make([]string, 0, 16)
	for _, part := range strings.Split(CSPPolicy(), "; ") {
		directive := part[:strings.Index(part, " ")]
		drop := false
		for _, bad := range metaUnsafeDirectives() {
			if directive == bad {
				drop = true
				break
			}
		}
		if !drop {
			keep = append(keep, part)
		}
	}
	return strings.Join(keep, "; ")
}
