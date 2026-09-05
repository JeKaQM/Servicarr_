package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetadataTargetsRejectedBeforeConnecting(t *testing.T) {
	for _, host := range []string{"169.254.169.254", "169.254.170.2", "169.254.170.23", "100.100.100.200", "[fd00:ec2::254]", "[fd00:ec2::23]", "metadata.google.internal."} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		conn, err := dialTargetContext(ctx, "tcp", host+":80")
		cancel()
		if conn != nil {
			conn.Close()
		}
		if err == nil {
			t.Fatalf("metadata target allowed: %s", host)
		}
	}
}

func TestCrossOriginRedirectDoesNotForwardCredentials(t *testing.T) {
	var reached atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true); w.WriteHeader(200) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer source.Close()
	ok, _, _, _ := Check(CheckOptions{URL: source.URL, APIToken: "secret-token", ServiceType: "sonarr", CheckType: "http", Timeout: time.Second, ExpectedMin: 200, ExpectedMax: 399})
	if ok || reached.Load() {
		t.Fatal("credential-bearing redirect reached another origin")
	}
}

func TestSameOriginRedirectPreservesPlexHeaderWithoutQueryToken(t *testing.T) {
	var verified atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.Header.Get("X-Plex-Token") != "secret-token" {
			t.Error("Plex credential handling incorrect")
		}
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/health", http.StatusFound)
			return
		}
		verified.Store(true)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	ok, _, _, _ := Check(CheckOptions{URL: srv.URL, APIToken: "secret-token", ServiceType: "plex", CheckType: "http", Timeout: time.Second, ExpectedMin: 200, ExpectedMax: 299})
	if !ok || !verified.Load() {
		t.Fatal("same-origin check failed")
	}
}
