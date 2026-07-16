package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebEndpoints(t *testing.T) {
	t.Parallel()

	server := &proxyServer{wsPath: defaultWSPath, maxMessageBytes: 1024}

	t.Run("index", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
		if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
			t.Fatalf("Content-Type = %q, want HTML", contentType)
		}
		if !strings.Contains(response.Body.String(), `src="/assets/js/main.js"`) {
			t.Fatal("index does not reference the JavaScript asset")
		}
	})

	t.Run("javascript", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://example.test/assets/js/main.js", nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
		if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/javascript") {
			t.Fatalf("Content-Type = %q, want JavaScript", contentType)
		}
		if !strings.Contains(response.Body.String(), `fetch("/healthz"`) {
			t.Fatal("JavaScript does not check the health endpoint")
		}
	})

	t.Run("head", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodHead, "http://example.test/", nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
		if response.Body.Len() != 0 {
			t.Fatalf("HEAD body length = %d, want 0", response.Body.Len())
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "http://example.test/", nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
		}
		if allow := response.Header().Get("Allow"); allow != "GET, HEAD" {
			t.Fatalf("Allow = %q, want GET, HEAD", allow)
		}
	})
}

func TestWebSocketUpgradeTakesPriorityOverJavaScript(t *testing.T) {
	t.Parallel()

	server := &proxyServer{wsPath: defaultWSPath, maxMessageBytes: 1024}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/assets/js/main.js", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef")))
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want WebSocket handler status %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), mainJavaScript) {
		t.Fatal("WebSocket upgrade request was served as static JavaScript")
	}
}
