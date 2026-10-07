package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// messageDTO is opencode's MessageWithParts: the four required Message fields
// under info, and the parts beside it. The flat id/sessionID/role/time half is
// gone because the client's decoder reads them from info.
type messageDTO struct {
	Info  messageInfoDTO `json:"info"`
	Parts []partDTO      `json:"parts"`
}

type messageInfoDTO struct {
	ID        string      `json:"id"`
	SessionID string      `json:"sessionID"`
	Role      string      `json:"role"`
	Time      messageTime `json:"time"`
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
	SessionID string `json:"sessionID"`
	Type      string `json:"type"`
}

type agentInfoDTO struct {
	Name string `json:"name"`
}

type projectDTO struct {
	ID       string `json:"id"`
	Worktree string `json:"worktree"`
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

// configDTO is the GET /config subset wopr can honestly report. model is the
// provider/model string the server starts sessions with; omitempty yields {}.
type configDTO struct {
	Model string `json:"model,omitempty"`
}

// apiCommandDTO is the GET /api/command body. Data is initialized to []any{} in
// the handler so it marshals [], never null; there is no element type because
// built-in slash commands are out of contract.
type apiCommandDTO struct {
	Location commandLocationDTO `json:"location"`
	Data     []any              `json:"data"`
}

type commandLocationDTO struct {
	Directory string                    `json:"directory"`
	Project   commandLocationProjectDTO `json:"project"`
}

type commandLocationProjectDTO struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────

const (
	tailBlockBytes       = 64 << 10
	maxTailBytes         = 16 << 20
	metaProbeBytes       = 32 << 10
	titleProbeBytes      = 8 << 10
	maxBodyBytes         = 16 << 20
	maxFileContentBytes  = 8 << 20
	maxFileEntries       = 500
	maxWalkEntries       = 20000
	maxFileStatusEntries = 500
	maxListFanOut        = 100 // cap concurrent goroutines for listSessions (MAJOR 4)
)

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// partID is the single source of part identity. The SSE mapper and
// entriesToMessages both call it, so a streamed part ID and a transcript part
// ID cannot drift.
//
// index is the position in the message's full content slice, enumerated before
// any block is filtered out. A block that yields no part still consumes its
// index, so the transcript's index equals ai.*Event.ContentIndex even where
// entriesToMessages has no case (ai.ServerToolContent) and the stream does.
// callID is the tool form's identity and is ignored by the other kinds.
func partID(messageID, kind string, index int, callID string) string {
	if kind == "tool" {
		return messageID + "-tool-" + callID
	}
	return messageID + "-" + kind + "-" + strconv.Itoa(index)
}

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
	// Reject absolute paths immediately (MAJOR: absolute paths silently reinterpreted as root-relative).
	if filepath.IsAbs(want) {
		return "", errOutsideRoot
	}

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
	if err != nil {
		return "", errOutsideRoot
	}
	// MAJOR: strings.HasPrefix(rel, "..") false-positives on legitimate names like "..foo".
	// Use exact match or prefix with separator.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errOutsideRoot
	}

	return resolved, nil
}

var (
	errMissingPath    = errors.New("path not found")
	errUnreadableLink = errors.New("unreadable or broken symlink")
	errOutsideRoot    = errors.New("path escapes root")
)
