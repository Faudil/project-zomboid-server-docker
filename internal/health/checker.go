package health

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/faudil/project-zomboid-server-docker/internal/server"
)

type Server struct {
	mu     sync.RWMutex
	status string
	srv    *server.Manager
}

func NewServer(srv *server.Manager) *Server {
	return &Server{
		status: "starting",
		srv:    srv,
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
	return http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
}
