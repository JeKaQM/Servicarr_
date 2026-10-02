package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A whole-CDN script source lets injected markup load arbitrary published
// code, which would defeat the policy. Keep CDNs out of script and connect.
func TestSecureHeadersKeepCDNsOutOfScriptSources(t *testing.T) {
	recorder := httptest.NewRecorder()
	SecureHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	csp := recorder.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("missing Content-Security-Policy")
	}
	for _, directive := range strings.Split(csp, ";") {
		directive = strings.TrimSpace(directive)
		name, _, _ := strings.Cut(directive, " ")
		if name != "script-src" && name != "connect-src" {
			continue
		}
		for _, banned := range []string{"jsdelivr", "unpkg", "cdnjs", "*", "'unsafe-inline'", "'unsafe-eval'", "data:"} {
			if strings.Contains(directive, banned) {
				t.Errorf("%s allows %q: %s", name, banned, directive)
			}
		}
	}
}
