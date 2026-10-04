package serve

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/alexrudloff/wopr/coding/version"
)

type healthResponse struct {
	Healthy bool   `json:"healthy"`
	Version string `json:"version"`
}

// New binds addr and returns the server and its listener, so a caller that
// wants the resolved address (an ephemeral test port) can reach it.
func New(addr string) (*http.Server, net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /global/health", health)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv, ln, nil
}

func health(w http.ResponseWriter, r *http.Request) {
	// healthy is unconditionally true. It means "this process is serving",
	// not "every dependency is ready". A dependency probe would make the
	// app's Test connection fail for reasons unrelated to reachability, and
	// the contract names no other field.
	resp := healthResponse{
		Healthy: true,
		Version: version.Version,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
