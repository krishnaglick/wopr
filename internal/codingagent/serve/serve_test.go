package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexrudloff/wopr/ai"
	"github.com/alexrudloff/wopr/ai/aitest"
	"github.com/alexrudloff/wopr/coding"
	codinghost "github.com/alexrudloff/wopr/coding/host"
	"github.com/alexrudloff/wopr/coding/version"
	"github.com/alexrudloff/wopr/internal/codingagent/web"
)

// TestMain sets up hermetic test environment.
func TestMain(m *testing.M) {
	// Mirror coding/host/host_test.go:22-59
	testRoot, err := os.MkdirTemp("", "wopr-serve-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create isolated test config root:", err)
		os.Exit(2)
	}
	// Unset WOPR_CODING_AGENT_DIR and WOPR_CODING_AGENT_SESSION_DIR
	for _, key := range []string{"WOPR_CODING_AGENT_DIR", "WOPR_CODING_AGENT_SESSION_DIR"} {
		if err := os.Unsetenv(key); err != nil {
			fmt.Fprintf(os.Stderr, "clear inherited %s: %v\n", key, err)
			_ = os.RemoveAll(testRoot)
			os.Exit(2)
		}
	}
	// Unset every key from ai.ProviderCredentialEnvVars() and web.SearchEnvVars()
	for _, v := range append(ai.ProviderCredentialEnvVars(), web.SearchEnvVars()...) {
		_ = os.Unsetenv(v)
	}
	// Apply aitest.HermeticCredentialEnv
	for key, value := range aitest.HermeticCredentialEnv(testRoot) {
		_ = os.Setenv(key, value)
	}
	// Point WOPR_HOME at a temp dir removed afterwards
	if err := os.Setenv("WOPR_HOME", filepath.Join(testRoot, "wopr")); err != nil {
		fmt.Fprintln(os.Stderr, "set isolated WOPR_HOME:", err)
		_ = os.RemoveAll(testRoot)
		os.Exit(2)
	}
	code := m.Run()
	if err := os.RemoveAll(testRoot); err != nil {
		fmt.Fprintln(os.Stderr, "remove isolated test config root:", err)
		if code == 0 {
			code = 2
		}
	}
	os.Exit(code)
}

// TestHealthAnswersOverARealListener verifies that GET /global/health returns
// 200 with the correct content type and body over a real loopback listener.
func TestHealthAnswersOverARealListener(t *testing.T) {
	// Create minimal services and host for health check.
	cwd := t.TempDir()
	agentDir := t.TempDir()
	services, err := coding.NewServices(coding.ServicesOptions{
		CWD:            cwd,
		AgentDir:       agentDir,
		ProjectTrusted: func() *bool { b := true; return &b }(),
	})
	if err != nil {
		t.Fatal(err)
	}
	host := codinghost.New(services, nil)

	srv, ln, err := New("127.0.0.1:0", host, "", "test-faux/faux-1")
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
	if contentType == "" || !strings.HasPrefix(contentType, "application/json") {
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
