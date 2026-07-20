package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleHealth(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	HandleHealth(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if body := recorder.Body.String(); body != "ok\n" {
		t.Fatalf("expected health response %q, got %q", "ok\n", body)
	}
}

func TestSetupRequiredMiddleware_AllowsHealthBeforeSetup(t *testing.T) {
	called := false
	handler := SetupRequiredMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, request)

	if !called {
		t.Fatal("expected health request to bypass setup redirect")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected wrapped handler status %d, got %d", http.StatusNoContent, recorder.Code)
	}
}
