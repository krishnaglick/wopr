package serve

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/alexrudloff/wopr/coding/version"
)

// TestHealthAnswersOverARealListener verifies that GET /global/health returns
// 200 with the correct content type and body over a real loopback listener.
func TestHealthAnswersOverARealListener(t *testing.T) {
	srv, ln, err := New("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	resp, err := http.Get("http://" + ln.Addr().String() + "/global/health")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || contentType[:len("application/json")] != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", contentType)
	}

	var body struct {
		Healthy bool   `json:"healthy"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if !body.Healthy {
		t.Fatalf("expected healthy=true, got %v", body.Healthy)
	}
	if body.Version != version.Version {
		t.Fatalf("expected version=%q, got %q", version.Version, body.Version)
	}
}
