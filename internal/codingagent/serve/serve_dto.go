package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ─── DTOs ────────────────────────────────────────────────────────────────

type sessionDTO struct {
	ID        string         `json:"id"`
	Slug      string         `json:"slug"`
	ProjectID string         `json:"projectID"`
	Directory string         `json:"directory"`
	Title     string         `json:"title"`
	Version   string         `json:"version"`
	Time      sessionTime    `json:"time"`
	Tokens    *sessionTokens `json:"tokens,omitempty"`
	Cost      *float64       `json:"cost,omitempty"`
}

type sessionTime struct {
	Created  int64  `json:"created"`
	Updated  int64  `json:"updated"`
	Archived *int64 `json:"archived,omitempty"`
}

type sessionTokens struct {
	Input  int        `json:"input"`
	Output int        `json:"output"`
	Cache  tokenCache `json:"cache"`
}

type tokenCache struct {
	Read  int `json:"read"`
	Write int `json:"write"`
}

type messageDTO struct {
	ID        string      `json:"id"`
	SessionID string      `json:"sessionID"`
	Role      string      `json:"role"`
	Time      messageTime `json:"time"`
	Parts     []partDTO   `json:"parts"`
}

type messageTime struct {
	Created int64 `json:"created"`
}

type partDTO struct {
	ID        string          `json:"id"`
	MessageID string          `json:"messageID"`
	SessionID string          `json:"sessionID"`
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	CallID    string          `json:"callID,omitempty"`
	Mime      string          `json:"mime,omitempty"`
	URL       string          `json:"url,omitempty"`
	State     json.RawMessage `json:"state,omitempty"`
}

type sessionStatusDTO struct {
	Type string `json:"type"`
}

type agentInfoDTO struct {
	Name string `json:"name"`
}

type projectDTO struct {
	ID       string `json:"id"`
	Worktree string `json:"worktree"`
}

type todoItemDTO struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

type fileNodeDTO struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

type fileStatusDTO struct {
	Path string `json:"path"`
}

type providerModelDTO struct {
	Name string `json:"name,omitempty"`
}

type providerDTO struct {
	Name   string                      `json:"name,omitempty"`
	API    string                      `json:"api,omitempty"`
	Models map[string]providerModelDTO `json:"models"`
}

type errorDTO struct {
	Error string `json:"error"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────

const (
	tailBlockBytes       = 64 << 10
	maxTailBytes         = 16 << 20
	metaProbeBytes       = 32 << 10
	titleProbeBytes      = 8 << 10
	maxBodyBytes         = 16 << 20
	maxFileEntries       = 500
	maxWalkEntries       = 20000
	maxFileStatusEntries = 500
	maxListFanOut        = 100 // cap concurrent goroutines for listSessions (MAJOR 4)
)

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// projectID returns the first 12 hex chars of sha256(root).
func projectID(root string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(root))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// resolveInRoot resolves want against root, refusing anything that leaves it.
// Symlinks are resolved on both sides first, so a link out of the project is a
// refusal rather than a read.
func resolveInRoot(root, want string) (string, error) {
	cleaned := filepath.Clean(want)
	cleaned = strings.TrimPrefix(cleaned, "/")
	candidate := filepath.Join(root, cleaned)

	if _, err := os.Lstat(candidate); err != nil {
		if os.IsNotExist(err) {
			return "", errMissingPath
		}
		return "", errUnreadableLink
	}

	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", errUnreadableLink
	}

	rel, err := filepath.Rel(root, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errOutsideRoot
	}

	return resolved, nil
}

var (
	errMissingPath    = errors.New("path not found")
	errUnreadableLink = errors.New("unreadable or broken symlink")
	errOutsideRoot    = errors.New("path escapes root")
)
