package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Server struct {
	mu     sync.RWMutex
	status string
	http   *http.Server
}

func NewServer() *Server {
	return &Server{
		status: "starting",
	}
}

func (s *Server) SetStatus(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	status := s.status
	s.mu.RUnlock()

	if r.URL.Path == "/health" || r.URL.Path == "/healthcheck" {
		if status != "healthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"%s"}`, status)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) ListenAndServe(port int) error {
	mux := http.NewServeMux()
	mux.Handle("/", s)
	s.http = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s.http.ListenAndServe()
}

// Shutdown stops the HTTP server with a short grace period so in-flight
// health probes complete.
func (s *Server) Shutdown() {
	if s.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.http.Shutdown(ctx)
}
