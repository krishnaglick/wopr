package serve

import (
	"encoding/json"
	"fmt"
	"io"
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

// promptServer starts a serve server over a test-faux host and returns its base
// URL. WOPR_TEST_FAUX=1 is required: BuildModel refuses test-faux without it,
// so createSession would answer 500 instead of the code under test.
func promptServer(t *testing.T) string {
	t.Helper()
	t.Setenv("WOPR_TEST_FAUX", "1")

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
	sessionDir := filepath.Join(agentDir, "sessions", "--"+strings.ReplaceAll(dir, "/", "-")+"--")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	host := codinghost.New(services, nil)
	srv, ln, err := New("127.0.0.1:0", host, sessionDir, "test-faux/faux-1")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "http://" + ln.Addr().String()
}

// createPromptSession creates a live session and returns its id.
func createPromptSession(t *testing.T, baseURL string) string {
	t.Helper()
	resp, err := http.Post(baseURL+"/session", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /session: expected 200, got %d", resp.StatusCode)
	}
	var dto sessionDTO
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		t.Fatal(err)
	}
	return dto.ID
}

// postJSON sends a body to path and returns the status and the whole response
// body. It drains and closes the body itself, so a caller cannot leak one.
func postJSON(t *testing.T, baseURL, path, body string) (int, []byte) {
	t.Helper()
	resp, err := http.Post(baseURL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, payload
}

// TestPromptAsyncAnswers204ForALiveSession verifies the accepted-prompt
// contract: 204, no body, and the run actually happened after the response was
// written — the detached context, not the request's.
func TestPromptAsyncAnswers204ForALiveSession(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)

	status, body := postJSON(t, baseURL, "/session/"+id+"/prompt_async",
		`{"parts":[{"type":"text","text":"reply with exactly: async-ok"}]}`)
	if status != http.StatusNoContent {
		t.Fatalf("prompt_async: expected 204, got %d", status)
	}
	if len(body) != 0 {
		t.Errorf("prompt_async: expected an empty body, got %q", body)
	}

	// The turn must still be running or finished after 204. An r.Context()
	// passed into the run would have killed it and left the transcript empty.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		msgs := readTranscript(t, baseURL, id)
		for _, m := range msgs {
			if m.Info.Role == "assistant" && len(m.Parts) > 0 && m.Parts[0].Text == "async-ok" {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the run did not reach the transcript after prompt_async answered 204")
}

// TestPromptAsyncAnswers404ForAnUnknownSession verifies the not-live case.
func TestPromptAsyncAnswers404ForAnUnknownSession(t *testing.T) {
	baseURL := promptServer(t)
	status, _ := postJSON(t, baseURL, "/session/no-such-session/prompt_async",
		`{"parts":[{"type":"text","text":"hi"}]}`)
	if status != http.StatusNotFound {
		t.Fatalf("prompt_async unknown session: expected 404, got %d", status)
	}
}

// TestPromptAsyncAnswers400ForAForeignDirectory verifies directoryParam applies
// to the prompt endpoints.
func TestPromptAsyncAnswers400ForAForeignDirectory(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)
	status, _ := postJSON(t, baseURL, "/session/"+id+"/prompt_async?directory=/somewhere/else",
		`{"parts":[{"type":"text","text":"hi"}]}`)
	if status != http.StatusBadRequest {
		t.Fatalf("prompt_async foreign directory: expected 400, got %d", status)
	}
}

// TestPromptAsyncAnswers400ForAMalformedBody verifies the malformed-body case,
// distinct from the oversize case.
func TestPromptAsyncAnswers400ForAMalformedBody(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)
	status, _ := postJSON(t, baseURL, "/session/"+id+"/prompt_async", `{"parts":`)
	if status != http.StatusBadRequest {
		t.Fatalf("prompt_async malformed body: expected 400, got %d", status)
	}
}

// TestPromptAsyncAnswers413ForABodyOverTheCap verifies the body cap on the
// endpoint that ingests base64 images from an unauthenticated LAN peer.
func TestPromptAsyncAnswers413ForABodyOverTheCap(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)
	huge := `{"parts":[{"type":"text","text":"` + strings.Repeat("x", maxBodyBytes+1024) + `"}]}`
	status, _ := postJSON(t, baseURL, "/session/"+id+"/prompt_async", huge)
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("prompt_async oversize body: expected 413, got %d", status)
	}
}

// TestPromptAsyncAnswers409WhileTheSessionIsBusy verifies one run per session:
// a second accepted prompt while the first is in flight is refused.
func TestPromptAsyncAnswers409WhileTheSessionIsBusy(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)

	// "Run: sleep for a while" gives a tool call that blocks long enough for
	// the contended request to land.
	first, _ := postJSON(t, baseURL, "/session/"+id+"/prompt_async",
		`{"parts":[{"type":"text","text":"Run: sleep for a while"}]}`)
	if first != http.StatusNoContent {
		t.Fatalf("first prompt_async: expected 204, got %d", first)
	}

	contended, _ := postJSON(t, baseURL, "/session/"+id+"/prompt_async",
		`{"parts":[{"type":"text","text":"reply with exactly: second"}]}`)
	if contended != http.StatusConflict {
		t.Fatalf("second prompt_async: expected 409, got %d", contended)
	}

	// Abort so the 45s tool does not outlive the test.
	abort, _ := postJSON(t, baseURL, "/session/"+id+"/abort", `{}`)
	if abort != http.StatusOK {
		t.Fatalf("abort: expected 200, got %d", abort)
	}
}

// TestMessageAnswers200WithTheFinishedAssistantMessage verifies /message's
// contract: 200 with the nested {info, parts} shape and the assistant reply.
func TestMessageAnswers200WithTheFinishedAssistantMessage(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)

	status, payload := postJSON(t, baseURL, "/session/"+id+"/message",
		`{"parts":[{"type":"text","text":"reply with exactly: message-ok"}],"system":"ignored","format":{},"agent":"build"}`)
	if status != http.StatusOK {
		t.Fatalf("POST /message: expected 200, got %d", status)
	}
	var dto messageDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		t.Fatalf("decode %q: %v", payload, err)
	}
	if dto.Info.Role != "assistant" {
		t.Errorf("info.role: expected assistant, got %q", dto.Info.Role)
	}
	if dto.Info.SessionID != id {
		t.Errorf("info.sessionID: expected %q, got %q", id, dto.Info.SessionID)
	}
	if dto.Info.ID == "" {
		t.Error("info.id: expected a transcript entry id, got empty")
	}
	if len(dto.Parts) != 1 || dto.Parts[0].Text != "message-ok" {
		t.Fatalf("parts: expected one text part %q, got %+v", "message-ok", dto.Parts)
	}
	// The literal form, not partID's own output: asserting against the helper
	// would pass for any helper that is self-consistent.
	if want := dto.Info.ID + "-text-0"; dto.Parts[0].ID != want {
		t.Errorf("parts[0].id: expected %q, got %q", want, dto.Parts[0].ID)
	}
}

// TestMessageAnswers404ForAnUnknownSession verifies the not-live case.
func TestMessageAnswers404ForAnUnknownSession(t *testing.T) {
	baseURL := promptServer(t)
	status, _ := postJSON(t, baseURL, "/session/no-such-session/message",
		`{"parts":[{"type":"text","text":"hi"}]}`)
	if status != http.StatusNotFound {
		t.Fatalf("POST /message unknown session: expected 404, got %d", status)
	}
}

// TestMessageAnswers413ForABodyOverTheCap verifies the body cap on /message.
func TestMessageAnswers413ForABodyOverTheCap(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)
	huge := `{"parts":[{"type":"file","url":"data:image/png;base64,` + strings.Repeat("A", maxBodyBytes+1024) + `"}]}`
	status, _ := postJSON(t, baseURL, "/session/"+id+"/message", huge)
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST /message oversize body: expected 413, got %d", status)
	}
}

// TestMessageAnswers400ForAnUnresolvableModel verifies the model picker cannot
// get a 200 that changed nothing.
func TestMessageAnswers400ForAnUnresolvableModel(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)
	status, _ := postJSON(t, baseURL, "/session/"+id+"/message",
		`{"parts":[{"type":"text","text":"hi"}],"model":{"providerID":"nope","modelID":"nope"}}`)
	if status != http.StatusBadRequest {
		t.Fatalf("POST /message unknown model: expected 400, got %d", status)
	}
}

// TestAbortAnswers200ForAnIdleSession verifies "stop nothing" is success.
func TestAbortAnswers200ForAnIdleSession(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)
	status, _ := postJSON(t, baseURL, "/session/"+id+"/abort", `{}`)
	if status != http.StatusOK {
		t.Fatalf("abort on idle session: expected 200, got %d", status)
	}
}

// TestAbortAnswers404ForAnUnknownSession verifies the not-live case.
func TestAbortAnswers404ForAnUnknownSession(t *testing.T) {
	baseURL := promptServer(t)
	status, _ := postJSON(t, baseURL, "/session/no-such-session/abort", `{}`)
	if status != http.StatusNotFound {
		t.Fatalf("abort unknown session: expected 404, got %d", status)
	}
}

// TestAbortStopsARunningPrompt verifies abort reaches a run whose reservation
// is outstanding, which coding's own cancel cannot: beginAgentRun has not run
// yet at the reservation.
func TestAbortStopsARunningPrompt(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)

	started, _ := postJSON(t, baseURL, "/session/"+id+"/prompt_async",
		`{"parts":[{"type":"text","text":"Run: sleep for a while"}]}`)
	if started != http.StatusNoContent {
		t.Fatalf("prompt_async: expected 204, got %d", started)
	}

	waitForBusy(t, baseURL, id)

	status, _ := postJSON(t, baseURL, "/session/"+id+"/abort", `{}`)
	if status != http.StatusOK {
		t.Fatalf("abort: expected 200, got %d", status)
	}
	waitForIdle(t, baseURL, id)
}

// TestSessionStatusCarriesSessionID verifies the pull-path status can be
// attributed to a session.
func TestSessionStatusCarriesSessionID(t *testing.T) {
	baseURL := promptServer(t)
	id := createPromptSession(t, baseURL)
	resp, err := http.Get(baseURL + "/session/status")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var dtos []sessionStatusDTO
	if err := json.NewDecoder(resp.Body).Decode(&dtos); err != nil {
		t.Fatal(err)
	}
	if len(dtos) != 1 {
		t.Fatalf("expected 1 status, got %d", len(dtos))
	}
	if dtos[0].SessionID != id {
		t.Errorf("sessionID: expected %q, got %q", id, dtos[0].SessionID)
	}
	if dtos[0].Type != "idle" {
		t.Errorf("type: expected idle, got %q", dtos[0].Type)
	}
}

// TestPartIDsAreUniqueWithinAMessage kills the serve-3 defect: the transcript
// loop enumerated content with no index, so two text blocks in one assistant
// message produced two parts with the same id. The fixture is written to the
// session file directly because test-faux has no two-same-kind-block script.
//
// The literal -text-1 and -text-2 also pin the index-before-filtering rule:
// ServerToolContent yields no part but still consumes index 0.
func TestPartIDsAreUniqueWithinAMessage(t *testing.T) {
	sessionDir := t.TempDir()
	id := "duplicate-parts"
	entryID := "0123456789abcdef"
	writeTestSession(t, sessionDir, id, []string{
		`{"type":"message","id":"` + entryID + `","parentId":null,"timestamp":"2024-01-01T00:00:01.000000000Z","message":{"role":"assistant","content":[` +
			`{"type":"serverTool","raw":{"type":"web_search_tool_result","content":[]}},` +
			`{"type":"text","text":"first"},` +
			`{"type":"text","text":"second"}` +
			`],"api":"faux","provider":"test-faux","model":"faux-1","usage":{"input":1,"output":1,"cache":{"read":0,"write":0}},"stopReason":"stop","timestamp":1704067201000}}`,
	})

	messages := readTranscriptFrom(t, sessionDir, id)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	got := make([]string, 0, len(messages[0].Parts))
	for _, p := range messages[0].Parts {
		got = append(got, p.ID)
	}
	want := []string{entryID + "-text-1", entryID + "-text-2"}
	if len(got) != len(want) {
		t.Fatalf("part ids: expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parts[%d].id: expected %q, got %q", i, want[i], got[i])
		}
	}
}

// TestPartIDsAgreeBetweenStreamAndTranscript pins that the SSE mapper and the
// transcript build the same id for the same (entryID, kind, index). It asserts
// the literal form the helper produces, not set equality: a duplicate collapses
// in a set on both sides, which is how the serve-3 defect passed green.
func TestPartIDsAgreeBetweenStreamAndTranscript(t *testing.T) {
	if partID("m1", "text", 1, "") != "m1-text-1" {
		t.Errorf("text form: expected m1-text-1, got %q", partID("m1", "text", 1, ""))
	}
	if partID("m1", "reasoning", 2, "") != "m1-reasoning-2" {
		t.Errorf("reasoning form: expected m1-reasoning-2, got %q", partID("m1", "reasoning", 2, ""))
	}
	if partID("m1", "image", 0, "") != "m1-image-0" {
		t.Errorf("image form: expected m1-image-0, got %q", partID("m1", "image", 0, ""))
	}
	if partID("m1", "tool", 3, "call-9") != "m1-tool-call-9" {
		t.Errorf("tool form: expected m1-tool-call-9, got %q", partID("m1", "tool", 3, "call-9"))
	}
	// The index is over the full content slice, so a leading block that yields
	// no part must still consume its index.
	sessionDir := t.TempDir()
	id := "index-alignment"
	entryID := "fedcba9876543210"
	writeTestSession(t, sessionDir, id, []string{
		`{"type":"message","id":"` + entryID + `","parentId":null,"timestamp":"2024-01-01T00:00:01.000000000Z","message":{"role":"assistant","content":[` +
			`{"type":"serverTool","raw":{"type":"web_search_tool_result","content":[]}},` +
			`{"type":"text","text":"after the unmapped block"}` +
			`],"api":"faux","provider":"test-faux","model":"faux-1","usage":{"input":1,"output":1,"cache":{"read":0,"write":0}},"stopReason":"stop","timestamp":1704067201000}}`,
	})
	messages := readTranscriptFrom(t, sessionDir, id)
	if len(messages) != 1 || len(messages[0].Parts) != 1 {
		t.Fatalf("expected 1 message with 1 part, got %d messages", len(messages))
	}
	if got, want := messages[0].Parts[0].ID, entryID+"-text-1"; got != want {
		t.Errorf("part id: expected %q (index over the full content slice), got %q", want, got)
	}
}

// readTranscript reads GET /session/{id}/message.
func readTranscript(t *testing.T, baseURL, id string) []messageDTO {
	t.Helper()
	resp, err := http.Get(baseURL + "/session/" + id + "/message")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var dtos []messageDTO
	if err := json.NewDecoder(resp.Body).Decode(&dtos); err != nil {
		t.Fatal(err)
	}
	return dtos
}

// readTranscriptFrom runs entriesToMessages over a written session file, which
// is the same projection GET /session/{id}/message serves.
func readTranscriptFrom(t *testing.T, sessionDir, id string) []messageDTO {
	t.Helper()
	path := filepath.Join(sessionDir, time.Now().UTC().Format("2006-01-02T15-04-05")+"_"+id+".jsonl")
	entries, err := tailEntries(path, maxLimit)
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{}
	return srv.entriesToMessages(entries, id)
}

func waitForBusy(t *testing.T, baseURL, id string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if !sessionIsIdle(t, baseURL, id) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the session never became busy")
}

func waitForIdle(t *testing.T, baseURL, id string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if sessionIsIdle(t, baseURL, id) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the session never returned to idle")
}

func sessionIsIdle(t *testing.T, baseURL, id string) bool {
	t.Helper()
	resp, err := http.Get(baseURL + "/session/status")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var dtos []sessionStatusDTO
	if err := json.NewDecoder(resp.Body).Decode(&dtos); err != nil {
		t.Fatal(err)
	}
	for _, dto := range dtos {
		if dto.SessionID == id {
			return dto.Type == "idle"
		}
	}
	t.Fatalf("session %q is absent from /session/status", id)
	return false
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
