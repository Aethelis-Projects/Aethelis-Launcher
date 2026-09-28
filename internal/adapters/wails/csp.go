package wails

import "net/http"

// CSPMiddleware pins the webview to the app's own assets. Solid renders via
// textContent (no innerHTML sink exists in src - verified in the v0.7.2 XSS
// audit), but a strict CSP is the defence-in-depth that makes the *next*
// injected-string bug insufficient to reach the IPC bridge: inline/eval
// scripts are refused, navigation and connections stay on 'self'. The wails
// beta.20 runtime bundle itself contains no eval/new Function (checked
// against bundledassets/runtime.js), so script-src needs no unsafe escape.
//
// img-src allows data: (avatar presets ship as data URIs); connect-src stays
// 'self': the webview performs no remote fetches - every network call runs in
// the Go core.
func CSPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; "+
				"object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
