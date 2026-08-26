package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func requestThrough(middleware gin.HandlerFunc, method, origin, authorization string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware)
	router.Any("/probe", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(method, "/probe", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestCORSDeniesUnknownOrigin(t *testing.T) {
	t.Setenv("TOUTIAO_ALLOWED_ORIGIN", "http://127.0.0.1")
	response := requestThrough(corsMiddleware(), http.MethodOptions, "https://evil.example", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want %d", response.Code, http.StatusForbidden)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin = %q, want empty", got)
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	t.Setenv("TOUTIAO_ALLOWED_ORIGIN", "http://127.0.0.1")
	response := requestThrough(corsMiddleware(), http.MethodOptions, "http://127.0.0.1", "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want %d", response.Code, http.StatusNoContent)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1" {
		t.Fatalf("allow-origin = %q", got)
	}
}

func TestWriteAuthRejectsMissingServerToken(t *testing.T) {
	t.Setenv("TOUTIAO_HTTP_TOKEN", "")
	response := requestThrough(writeAuthMiddleware(), http.MethodPost, "", "")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestWriteAuthRejectsWrongToken(t *testing.T) {
	t.Setenv("TOUTIAO_HTTP_TOKEN", "expected")
	response := requestThrough(writeAuthMiddleware(), http.MethodPost, "", "Bearer wrong")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestWriteAuthAcceptsBearerToken(t *testing.T) {
	t.Setenv("TOUTIAO_HTTP_TOKEN", "expected")
	response := requestThrough(writeAuthMiddleware(), http.MethodPost, "", "Bearer expected")
	if response.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want %d", response.Code, http.StatusNoContent)
	}
}
