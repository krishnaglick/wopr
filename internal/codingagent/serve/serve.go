package serve

import (
	"net"
	"net/http"
	"time"

	"github.com/alexrudloff/wopr/coding/host"
)

type healthResponse struct {
	Healthy bool   `json:"healthy"`
	Version string `json:"version"`
}

// Server holds the HTTP server and its dependencies.
type Server struct {
	services   *host.Services
	host       *host.Host
	sessionDir string
	modelSpec  string
	srv        *http.Server
	ln         net.Listener
}

// New binds addr and returns the server and its listener, so a caller that
// wants the resolved address (an ephemeral test port) can reach it.
func New(addr string, h *host.Host, sessionDir, modelSpec string) (*http.Server, net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	s := &Server{
		services:   h.Services(),
		host:       h,
		sessionDir: sessionDir,
		modelSpec:  modelSpec,
		ln:         ln,
	}

	mux := http.NewServeMux()
	// Health
	mux.HandleFunc("GET /global/health", s.health)

	// Sessions
	mux.HandleFunc("GET /session", s.listSessions)
	mux.HandleFunc("POST /session", s.createSession)
	mux.HandleFunc("GET /session/{id}", s.getSession)
	mux.HandleFunc("PATCH /session/{id}", s.patchSession)
	mux.HandleFunc("DELETE /session/{id}", s.notImplemented)
	mux.HandleFunc("POST /session/{id}/fork", s.notImplemented)
	mux.HandleFunc("POST /session/{id}/revert", s.notImplemented)
	mux.HandleFunc("GET /session/{id}/diff", s.notImplemented)
	mux.HandleFunc("GET /session/{id}/todo", s.sessionTodo)
	mux.HandleFunc("GET /session/{id}/message", s.sessionMessages)
	mux.HandleFunc("GET /session/status", s.sessionStatus)
	mux.HandleFunc("POST /session/{id}/prompt_async", s.promptAsync)
	mux.HandleFunc("POST /session/{id}/message", s.sessionMessage)
	mux.HandleFunc("POST /session/{id}/abort", s.sessionAbort)

	// Project
	mux.HandleFunc("GET /project", s.listProjects)
	mux.HandleFunc("GET /project/current", s.getCurrentProject)

	// Config
	mux.HandleFunc("GET /config/providers", s.listProviders)

	// Providers
	mux.HandleFunc("GET /provider", s.listProviderNames)

	// Agents
	mux.HandleFunc("GET /agent", s.listAgents)

	// Files
	mux.HandleFunc("GET /file", s.listFiles)
	mux.HandleFunc("GET /find/file", s.findFiles)
	mux.HandleFunc("GET /file/status", s.fileStatus)

	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s.srv, s.ln, nil
}

// Services returns the coding.Services for testing.
func (s *Server) Services() *host.Services {
	return s.services
}
