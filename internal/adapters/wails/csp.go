package wails

import "net/http"

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

// CSPPolicy is the single source of truth; frontend/index.html keeps a
// byte-equivalent <meta http-equiv> fallback for webview backends that do not
// surface asset-server response headers (notably some webkit2gtk
// custom-scheme paths). The byte-equality is unit-tested (csp_test.go).
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
