package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateWebSocketRequest(t *testing.T) {
	t.Parallel()

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	request := httptest.NewRequest(http.MethodGet, "http://example.test/ws", nil)
	request.Header.Set("Connection", "keep-alive, Upgrade")
	request.Header.Set("Upgrade", "WebSocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", key)

	got, err := validateWebSocketRequest(request)
	if err != nil {
		t.Fatalf("validateWebSocketRequest: %v", err)
	}
	if got != key {
		t.Fatalf("key = %q, want %q", got, key)
	}

	request.Header.Del("Connection")
	if _, err := validateWebSocketRequest(request); err == nil {
		t.Fatal("missing Connection token unexpectedly accepted")
	}
}

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	server := &proxyServer{wsPath: "/ws", maxMessageBytes: 1024}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/healthz", nil)
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != "ok\n" {
		t.Fatalf("health response = %d %q", response.Code, response.Body.String())
	}
}
