package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestListSessionsReturnsAllSessions verifies that GET /session returns all
// sessions on disk with the correct IDs. Runs with -count=20 to catch races.
func TestListSessionsReturnsAllSessions(t *testing.T) {
	dir := t.TempDir()
	agentDir := t.TempDir()

	services, err := coding.NewServices(coding.ServicesOptions{
		CWD:            dir,
		AgentDir:       agentDir,
		ProjectTrusted: func() *bool { b := true; return &b }(),
	})
	if err != nil {
		t.Fatal(err)
	}
	host := codinghost.New(services, nil)

	sessionDir := filepath.Join(agentDir, "sessions", "--test--")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create 5 session files on disk with different IDs.
	ids := make([]string, 5)
	for i := range 5 {
		id := fmt.Sprintf("session-%02d", i)
		ids[i] = id
		path := filepath.Join(sessionDir, time.Now().UTC().Format("2006-01-02T15-04-05")+"_"+id+".jsonl")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		header := `{"type":"session","version":3,"id":"` + id + `","timestamp":"2024-01-01T00:00:00.000000000Z","cwd":"/test"}`
		_, _ = f.WriteString(header + "\n")
		entry := `{"type":"message","id":"e1","parentId":null,"timestamp":"2024-01-01T00:00:01.000000000Z","message":{"role":"user","content":[{"type":"text","text":"hi"}],"timestamp":1704067201000}}`
		_, _ = f.WriteString(entry + "\n")
		_ = f.Close()
	}

	srv, ln, err := New("127.0.0.1:0", host, sessionDir, "test-faux/faux-1")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	baseURL := "http://" + ln.Addr().String()

	resp, err := http.Get(baseURL + "/session")
	if err != nil {
		t.Fatal(err)
	}
	respBody := make([]byte, 4096)
	n, _ := resp.Body.Read(respBody)
	_ = resp.Body.Close()

	var sessions []map[string]any
	if err := json.Unmarshal(respBody[:n], &sessions); err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 5 {
		t.Fatalf("expected 5 sessions, got %d", len(sessions))
	}

	found := make(map[string]bool)
	for _, s := range sessions {
		id := s["id"].(string)
		found[id] = true
	}
	for _, id := range ids {
		if !found[id] {
			t.Errorf("expected session %q not found", id)
		}
	}
}
