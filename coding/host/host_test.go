package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alexrudloff/wopr/ai"
	"github.com/alexrudloff/wopr/ai/aitest"
	"github.com/alexrudloff/wopr/coding"
	"github.com/alexrudloff/wopr/internal/codingagent/web"
)

func TestMain(m *testing.M) {
	// Mirror coding/testmain_test.go:15-49
	testRoot, err := os.MkdirTemp("", "wopr-host-tests-")
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
	for _, key := range append(ai.ProviderCredentialEnvVars(), web.SearchEnvVars()...) {
		_ = os.Unsetenv(key)
	}
	// Apply aitest.HermeticCredentialEnv so no cloud SDK chain reaches instance metadata
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

// TestHostRejectsASecondConcurrentPromptPerSession verifies that with a prompt
// in flight on session A, A.Prompt returns ErrSessionBusy, while B.Prompt for a
// different session is accepted and reaches the provider.
func TestHostRejectsASecondConcurrentPromptPerSession(t *testing.T) {
	// Create a blocking HTTP server that signals on entry and blocks until released
	entryCh := make(chan struct{}, 1)
	releaseCh := make(chan struct{})
	requestCount := 0
	blockingSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		entryCh <- struct{}{}
		<-releaseCh
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range []string{
			`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"test-model","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}`,
			`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
			`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop","index":0}`,
			`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
		} {
			_, _ = io.WriteString(w, line+"\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer blockingSrv.Close()

	// Create temp dir for WOPR_HOME and models.json
	tempDir := t.TempDir()
	agentDir := filepath.Join(tempDir, "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a models.json with anthropic entry pointing to our blocking server
	modelsContent := `{"providers":{"anthropic":{"baseUrl":"` + blockingSrv.URL + `","apiKey":"test-key","modelOverrides":{"test-model":{"promptCache":{"short":12}}}}}}`
	modelsJSON := filepath.Join(agentDir, "models.json")
	if err := os.WriteFile(modelsJSON, []byte(modelsContent), 0o644); err != nil {
		t.Fatalf("write models.json: %v", err)
	}

	// Build services with our temp dir
	svcs, err := coding.NewServices(coding.ServicesOptions{
		AgentDir: agentDir,
	})
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}

	// Build model by spec using our test server
	model, err := coding.BuildModel("anthropic/test-model", svcs)
	if err != nil {
		t.Fatalf("BuildModel: %v", err)
	}
	t.Logf("Built model: %s provider: %s", model.ID, model.Provider.ID())

	// Create host
	h := New(svcs, nil)

	// Start session A
	sessA, err := h.Start(StartOptions{Model: "anthropic/test-model"})
	if err != nil {
		t.Fatalf("Start session A: %v", err)
	}
	defer func() { _ = sessA.Close() }()

	// Start session B
	sessB, err := h.Start(StartOptions{Model: "anthropic/test-model"})
	if err != nil {
		t.Fatalf("Start session B: %v", err)
	}
	defer func() { _ = sessB.Close() }()

	// Start a prompt on session A that will block
	var wg sync.WaitGroup
	wg.Go(func() {
		// This will block until releaseCh is closed
		_ = sessA.Prompt(context.Background(), "first prompt")
	})

	// Wait for the request to reach the blocking server
	select {
	case <-entryCh:
		// Good, the first request is in flight
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first request to reach server")
	}

	// Now try a second prompt on session A - should return ErrSessionBusy immediately
	err = sessA.Prompt(context.Background(), "second prompt")
	if !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("expected ErrSessionBusy, got %v", err)
	}

	// Now try a prompt on session B - should be accepted and reach the provider
	var sessBErr error
	wg.Go(func() {
		sessBErr = sessB.Prompt(context.Background(), "prompt on B")
	})

	// Wait for session B's request to reach the server
	select {
	case <-entryCh:
		// Session B's request also reached the server
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for session B request to reach server")
	}

	// Release the blocking server so both requests can complete
	close(releaseCh)

	// Wait for both goroutines to finish
	wg.Wait()

	// Session B's prompt should not have returned ErrSessionBusy
	if errors.Is(sessBErr, ErrSessionBusy) {
		t.Fatalf("session B should not be busy, got %v", sessBErr)
	}
}
