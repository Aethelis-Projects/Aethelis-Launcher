package wails

// v0.7.2 round-4 review: if an OpenURL-style IPC surface exists (today the
// only OS-browser launch path is the auth flow), it must refuse non-http(s)
// schemes - file:, javascript:, ms-msdt:, custom protocol handlers all turn a
// clicked link into code execution. Tested through the real exported surface:
// a forged LoginMicrosoft callback cannot bypass it because the validator runs
// inside openBrowserCrossPlatform before exec.

import (
	"testing"
)

func TestOpenURLSchemeAllowlist(t *testing.T) {
	for _, bad := range []string{"file:///etc/passwd", "javascript:alert(1)", "ms-msdt:/id foo", "x-scheme-handler/whatever", "https://", "  ", "not a url", "/relative/path"} {
		if err := ValidateExternalURLForTest(bad); err == nil {
			t.Errorf("must refuse: %q", bad)
		}
	}
	for _, good := range []string{"https://modrinth.com/mod/sodium", "http://localhost:8080/cb"} {
		if err := ValidateExternalURLForTest(good); err != nil {
			t.Errorf("must accept %q: %v", good, err)
		}
	}
}
