package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRejectCrossOriginMutations(t *testing.T) {
	h := SecureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, tc := range []struct {
		method, origin, site string
		want                 int
	}{
		{http.MethodPost, "https://attacker.example", "", 403},
		{http.MethodPost, "null", "", 403},
		{http.MethodPost, "", "cross-site", 403},
		{http.MethodPost, "https://status.example", "same-origin", 204},
		{http.MethodPost, "", "", 204},
		{http.MethodGet, "https://attacker.example", "cross-site", 204},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, "https://status.example/api/setup/complete", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%+v: got %d", tc, w.Code)
		}
	}
}
