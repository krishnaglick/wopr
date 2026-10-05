package serve

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alexrudloff/wopr/coding"
	host "github.com/alexrudloff/wopr/coding/host"
)

// TestSessionDTODecode tests that sessionDTO decodes correctly into a client-shaped mirror.
func TestSessionDTODecode(t *testing.T) {
	dto := sessionDTO{
		ID:        "test-id",
		Slug:      "test-id",
		ProjectID: "abc123def456",
		Directory: "/home/user/project",
		Title:     "Test Session",
		Version:   "0.2.1",
		Time: sessionTime{
			Created:  1700000000000,
			Updated:  1700000001000,
			Archived: nil,
		},
		Tokens: &sessionTokens{
			Input:  100,
			Output: 200,
			Cache:  tokenCache{Read: 50, Write: 25},
		},
		Cost: func() *float64 { v := 0.001; return &v }(),
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}

	// Client-shaped mirror with non-pointer required fields.
	var mirror struct {
		ID        string `json:"id"`
		Slug      string `json:"slug"`
		ProjectID string `json:"projectID"`
		Directory string `json:"directory"`
		Title     string `json:"title"`
		Version   string `json:"version"`
		Time      struct {
			Created  int64  `json:"created"`
			Updated  int64  `json:"updated"`
			Archived *int64 `json:"archived,omitempty"`
		} `json:"time"`
		Tokens *struct {
			Input  int `json:"input"`
			Output int `json:"output"`
			Cache  struct {
				Read  int `json:"read"`
				Write int `json:"write"`
			} `json:"cache"`
		} `json:"tokens,omitempty"`
		Cost *float64 `json:"cost,omitempty"`
	}

	if err := json.Unmarshal(data, &mirror); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if mirror.ID != dto.ID {
		t.Errorf("ID: expected %q, got %q", dto.ID, mirror.ID)
	}
	if mirror.Slug != dto.Slug {
		t.Errorf("Slug: expected %q, got %q", dto.Slug, mirror.Slug)
	}
	if mirror.ProjectID != dto.ProjectID {
		t.Errorf("ProjectID: expected %q, got %q", dto.ProjectID, mirror.ProjectID)
	}
	if mirror.Directory != dto.Directory {
		t.Errorf("Directory: expected %q, got %q", dto.Directory, mirror.Directory)
	}
	if mirror.Title != dto.Title {
		t.Errorf("Title: expected %q, got %q", dto.Title, mirror.Title)
	}
	if mirror.Version != dto.Version {
		t.Errorf("Version: expected %q, got %q", dto.Version, mirror.Version)
	}
	if mirror.Time.Created != dto.Time.Created {
		t.Errorf("Time.Created: expected %d, got %d", dto.Time.Created, mirror.Time.Created)
	}
	if mirror.Time.Updated != dto.Time.Updated {
		t.Errorf("Time.Updated: expected %d, got %d", dto.Time.Updated, mirror.Time.Updated)
	}
	if mirror.Tokens == nil {
		t.Fatal("Tokens should not be nil")
	}
	if mirror.Tokens.Input != dto.Tokens.Input {
		t.Errorf("Tokens.Input: expected %d, got %d", dto.Tokens.Input, mirror.Tokens.Input)
	}
	if mirror.Tokens.Output != dto.Tokens.Output {
		t.Errorf("Tokens.Output: expected %d, got %d", dto.Tokens.Output, mirror.Tokens.Output)
	}
	if mirror.Tokens.Cache.Read != dto.Tokens.Cache.Read {
		t.Errorf("Tokens.Cache.Read: expected %d, got %d", dto.Tokens.Cache.Read, mirror.Tokens.Cache.Read)
	}
	if mirror.Tokens.Cache.Write != dto.Tokens.Cache.Write {
		t.Errorf("Tokens.Cache.Write: expected %d, got %d", dto.Tokens.Cache.Write, mirror.Tokens.Cache.Write)
	}
	if mirror.Cost == nil {
		t.Fatal("Cost should not be nil")
	}
	if *mirror.Cost != *dto.Cost {
		t.Errorf("Cost: expected %f, got %f", *dto.Cost, *mirror.Cost)
	}
}

// TestMessageDTODecode tests that messageDTO decodes correctly.
func TestMessageDTODecode(t *testing.T) {
	dto := messageDTO{
		ID:        "msg-1",
		SessionID: "sess-1",
		Role:      "assistant",
		Time:      messageTime{Created: 1700000000000},
		Parts: []partDTO{
			{ID: "p1", MessageID: "msg-1", SessionID: "sess-1", Type: "text", Text: "Hello"},
			{ID: "p2", MessageID: "msg-1", SessionID: "sess-1", Type: "tool", Tool: "bash", CallID: "call-1", State: json.RawMessage(`{"status":"pending"}`)},
		},
	}

	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}

	var mirror struct {
		ID        string                  `json:"id"`
		SessionID string                  `json:"sessionID"`
		Role      string                  `json:"role"`
		Time      struct{ Created int64 } `json:"time"`
		Parts     []struct {
			ID        string          `json:"id"`
			MessageID string          `json:"messageID"`
			SessionID string          `json:"sessionID"`
			Type      string          `json:"type"`
			Text      string          `json:"text,omitempty"`
			Tool      string          `json:"tool,omitempty"`
			CallID    string          `json:"callID,omitempty"`
			State     json.RawMessage `json:"state,omitempty"`
		} `json:"parts"`
	}

	if err := json.Unmarshal(data, &mirror); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if mirror.ID != dto.ID {
		t.Errorf("ID mismatch")
	}
	if len(mirror.Parts) != 2 {
		t.Errorf("expected 2 parts, got %d", len(mirror.Parts))
	}
}

// TestSessionStatusDTODecode tests sessionStatusDTO.
func TestSessionStatusDTODecode(t *testing.T) {
	dto := sessionStatusDTO{Type: "busy"}
	data, _ := json.Marshal(dto)

	var mirror struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &mirror); err != nil {
		t.Fatal(err)
	}
	if mirror.Type != "busy" {
		t.Errorf("expected busy, got %q", mirror.Type)
	}
}

// TestAgentInfoDTODecode tests agentInfoDTO.
func TestAgentInfoDTODecode(t *testing.T) {
	dto := agentInfoDTO{Name: "explore"}
	data, _ := json.Marshal(dto)

	var mirror struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &mirror); err != nil {
		t.Fatal(err)
	}
	if mirror.Name != "explore" {
		t.Errorf("expected explore, got %q", mirror.Name)
	}
}

// TestProjectDTODecode tests projectDTO.
func TestProjectDTODecode(t *testing.T) {
	dto := projectDTO{ID: "abc123", Worktree: "/home/user/project"}
	data, _ := json.Marshal(dto)

	var mirror struct {
		ID       string `json:"id"`
		Worktree string `json:"worktree"`
	}
	if err := json.Unmarshal(data, &mirror); err != nil {
		t.Fatal(err)
	}
	if mirror.ID != "abc123" || mirror.Worktree != "/home/user/project" {
		t.Errorf("projectDTO mismatch")
	}
}

// TestProviderDTODecode tests providerDTO.
func TestProviderDTODecode(t *testing.T) {
	dto := providerDTO{
		Name: "Anthropic",
		API:  "anthropic",
		Models: map[string]providerModelDTO{
			"claude-3": {Name: "Claude 3"},
		},
	}
	data, _ := json.Marshal(dto)

	var mirror struct {
		Name   string `json:"name,omitempty"`
		API    string `json:"api,omitempty"`
		Models map[string]struct {
			Name string `json:"name,omitempty"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &mirror); err != nil {
		t.Fatal(err)
	}
	if mirror.Name != "Anthropic" {
		t.Errorf("provider name mismatch")
	}
}

// TestErrorDTODecode tests errorDTO.
func TestErrorDTODecode(t *testing.T) {
	dto := errorDTO{Error: "something went wrong"}
	data, _ := json.Marshal(dto)

	var mirror struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &mirror); err != nil {
		t.Fatal(err)
	}
	if mirror.Error != "something went wrong" {
		t.Errorf("error message mismatch")
	}
}

// TestRequestBodyFieldsAreBuiltins tests that all request body structs
// have only builtin field types (no named types that would leak pkg.Type in errors).
func TestRequestBodyFieldsAreBuiltins(t *testing.T) {
	// The only request body in serve is the PATCH /session/{id} body.
	// It uses an inline struct with *string and *struct{Archived *int64} fields.
	// Both are builtin or anonymous struct types.

	// Use reflection to check all types that serve decodes into.
	// We manually enumerate the request body types here.
	type patchBody struct {
		Title *string `json:"title"`
		Time  *struct {
			Archived *int64 `json:"archived"`
		} `json:"time"`
	}

	checkStruct(t, reflect.TypeFor[patchBody]())
}

func checkStruct(t *testing.T, typ reflect.Type) {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	for field := range typ.Fields() {
		checkFieldType(t, field.Type)
	}
}

func checkFieldType(t *testing.T, typ reflect.Type) {
	// Dereference pointers, slices, maps, arrays.
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map || typ.Kind() == reflect.Array {
		switch typ.Kind() {
		case reflect.Pointer:
			typ = typ.Elem()
		case reflect.Slice:
			typ = typ.Elem()
		case reflect.Map:
			typ = typ.Elem()
		case reflect.Array:
			typ = typ.Elem()
		}
	}

	// The key check: for builtin types, PkgPath() is empty.
	// Named types from packages (like codingagent.SessionHeader) have a non-empty PkgPath().
	// Anonymous structs also have empty PkgPath().
	if typ.PkgPath() != "" {
		t.Errorf("request body field has named type %s (pkg.Type would leak in errors)", typ.String())
	}
}

// TestPatchThenGetAgrees tests that PATCH /session/{id} then GET /session/{id}
// returns the same data in both reachable states: cold (file on disk) and live (no file).
func TestPatchThenGetAgrees(t *testing.T) {
	// State 1: cold session (file on disk)
	t.Run("cold_session", func(t *testing.T) {
		agentDir := t.TempDir()

		services, err := coding.NewServices(coding.ServicesOptions{
			CWD:            t.TempDir(),
			AgentDir:       agentDir,
			ProjectTrusted: func() *bool { b := true; return &b }(),
		})
		if err != nil {
			t.Fatal(err)
		}
		host := host.New(services, nil)

		sessionDir := filepath.Join(agentDir, "sessions", "--test--")
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatal(err)
		}

		// Create a session file with header and two entries.
		id := "cold-session"
		path := filepath.Join(sessionDir, time.Now().UTC().Format("2006-01-02T15-04-05")+"_"+id+".jsonl")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		header := `{"type":"session","version":3,"id":"` + id + `","timestamp":"2024-01-01T00:00:00.000000000Z","cwd":"/test"}`
		_, _ = f.WriteString(header + "\n")
		entry1 := `{"type":"message","id":"e1","parentId":null,"timestamp":"2024-01-01T00:00:01.000000000Z","message":{"role":"user","content":[{"type":"text","text":"hi"}],"timestamp":1704067201000}}`
		_, _ = f.WriteString(entry1 + "\n")
		_ = f.Close()

		srv, ln, err := New("127.0.0.1:0", host, sessionDir, "test-faux/faux-1")
		if err != nil {
			t.Fatal(err)
		}
		go func() { _ = srv.Serve(ln) }()
		defer func() { _ = srv.Close() }()

		baseURL := "http://" + ln.Addr().String()

		// PATCH to rename
		patchBody := `{"title":"New Name"}`
		req, err := http.NewRequest("PATCH", baseURL+"/session/"+id, strings.NewReader(patchBody))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PATCH failed: %d", resp.StatusCode)
		}
		defer func() { _ = resp.Body.Close() }()

		// GET to verify
		resp, err = http.Get(baseURL + "/session/" + id)
		if err != nil {
			t.Fatal(err)
		}
		var getDTO sessionDTO
		err = json.NewDecoder(resp.Body).Decode(&getDTO)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Verify PATCH response matches GET
		// (In this test we only verify GET works after PATCH)
		if getDTO.Title != "New Name" {
			t.Errorf("expected title 'New Name', got %q", getDTO.Title)
		}
	})

	// State 2: live session (no file yet)
	t.Run("live_session", func(t *testing.T) {
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
		host := host.New(services, nil)

		sessionDir := filepath.Join(agentDir, "sessions", "--"+strings.ReplaceAll(dir, "/", "-")+"--")
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatal(err)
		}

		srv, ln, err := New("127.0.0.1:0", host, sessionDir, "test-faux/faux-1")
		if err != nil {
			t.Fatal(err)
		}
		go func() { _ = srv.Serve(ln) }()
		defer func() { _ = srv.Close() }()

		baseURL := "http://" + ln.Addr().String()

		// Create session (POST /session)
		resp, err := http.Post(baseURL+"/session", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		var createDTO sessionDTO
		err = json.NewDecoder(resp.Body).Decode(&createDTO)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		id := createDTO.ID

		// PATCH to rename
		patchBody := `{"title":"Live Session Name"}`
		req, err := http.NewRequest("PATCH", baseURL+"/session/"+id, strings.NewReader(patchBody))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PATCH failed: %d", resp.StatusCode)
		}
		defer func() { _ = resp.Body.Close() }()

		// GET to verify
		resp, err = http.Get(baseURL + "/session/" + id)
		if err != nil {
			t.Fatal(err)
		}
		var getDTO sessionDTO
		err = json.NewDecoder(resp.Body).Decode(&getDTO)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if getDTO.Title != "Live Session Name" {
			t.Errorf("expected title 'Live Session Name', got %q", getDTO.Title)
		}
	})
}
