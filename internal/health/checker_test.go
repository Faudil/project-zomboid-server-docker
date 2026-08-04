package health

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func netListen() (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") }

func TestHealthStatusTransitions(t *testing.T) {
	h := NewServer()

	tests := []struct {
		status   string
		wantCode int
		wantBody string
	}{
		{"starting", http.StatusServiceUnavailable, `{"status":"starting"}`},
		{"installing", http.StatusServiceUnavailable, `{"status":"installing"}`},
		{"stopping", http.StatusServiceUnavailable, `{"status":"stopping"}`},
		{"healthy", http.StatusOK, `{"status":"healthy"}`},
	}
	for _, tt := range tests {
		h.SetStatus(tt.status)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != tt.wantCode {
			t.Errorf("status %q: got code %d, want %d", tt.status, rec.Code, tt.wantCode)
		}
		if body := strings.TrimSpace(rec.Body.String()); body != tt.wantBody {
			t.Errorf("status %q: got body %q, want %q", tt.status, body, tt.wantBody)
		}
	}
}

func TestHealthPaths(t *testing.T) {
	h := NewServer()
	h.SetStatus("healthy")

	for _, path := range []string{"/health", "/healthcheck"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("path %s: got code %d, want 200", path, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown path: got code %d, want 404", rec.Code)
	}
}

func TestListenAndServeAndShutdown(t *testing.T) {
	h := NewServer()
	h.SetStatus("healthy")

	// Find a free port, then serve on it.
	ln, err := netListen()
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- h.ListenAndServe(port) }()

	// Poll until the server is up.
	var resp *http.Response
	for i := 0; i < 50; i++ {
		client := &http.Client{Timeout: 200 * time.Millisecond}
		resp, err = client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != `{"status":"healthy"}` {
		t.Errorf("unexpected response: %d %q", resp.StatusCode, body)
	}

	h.Shutdown()
	select {
	case <-errCh:
	case <-time.After(3 * time.Second):
		t.Error("ListenAndServe did not return after Shutdown")
	}
}
