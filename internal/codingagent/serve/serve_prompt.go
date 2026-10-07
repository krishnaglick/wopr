package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alexrudloff/wopr/agent"
	"github.com/alexrudloff/wopr/ai"
	codinghost "github.com/alexrudloff/wopr/coding/host"
	icodingagent "github.com/alexrudloff/wopr/internal/codingagent"
)

// promptBody is the body both prompt endpoints accept. system, format and
// agent are declared nowhere: json.Decode ignores unknown keys, so a body
// without them accepts all three for free.
//
// ponytail: ignored per request, not per session. A per-prompt system
// override, a JSON-schema-constrained retry loop (opencode's
// StructuredOutputError path) and runtime agent personas are three different
// slices; declaring the fields now would promise behaviour nothing implements.
type promptBody struct {
	Parts []promptPart `json:"parts"`
	Model *modelRef    `json:"model,omitempty"`
}

// promptPart is one client part. Text and file are sent; every other type the
// client may send (step markers, reasoning, tool state) carries nothing the
// session can be prompted with and is ignored.
type promptPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Mime string `json:"mime,omitempty"`
	URL  string `json:"url,omitempty"`
}

// modelRef is opencode's model reference.
type modelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// POST /session/{id}/prompt_async
//
// 204 with no body once the session's single run slot is reserved; 404 for a
// session this server did not start; 409 while a run is in flight; 400 for a
// directory that is not the server root or a malformed body; 413 for a body
// over maxBodyBytes.
func (s *Server) promptAsync(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionPreconditions(w, r)
	if !ok {
		return
	}
	var body promptBody
	if !s.decodePromptBody(w, r, &body) {
		return
	}
	content, errDTO := bodyContent(body.Parts)
	if errDTO != nil {
		s.writeJSON(w, http.StatusBadRequest, errDTO)
		return
	}
	if err := sess.PromptContent(r.Context(), content); err != nil {
		s.writePromptError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /session/{id}/message
//
// 200 with the finished assistant message as {info, parts}. 404, 409, 400 and
// 413 as for prompt_async, plus 400 for a model this server does not offer.
func (s *Server) sessionMessage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionPreconditions(w, r)
	if !ok {
		return
	}
	var body promptBody
	if !s.decodePromptBody(w, r, &body) {
		return
	}
	content, errDTO := bodyContent(body.Parts)
	if errDTO != nil {
		s.writeJSON(w, http.StatusBadRequest, errDTO)
		return
	}

	type outcome struct {
		entryID  string
		runError error
		// errDTO and status carry a refusal from before the turn started, which
		// is a client's own bad model rather than a failure of the run.
		errDTO *errorDTO
		status int
	}
	settled := make(chan outcome, 1)
	go func() {
		// The model is a session-wide mutation, so it is applied once the run
		// slot is held and before the turn starts: a busy session is answered
		// 409 with its model untouched, and a turn never runs on the model the
		// previous request chose.
		var errDTO *errorDTO
		var status int
		msgs, runError := sess.PromptContentWait(r.Context(), content, func() error {
			if body.Model == nil {
				return nil
			}
			errDTO, status = s.applyModel(sess, *body.Model)
			if errDTO != nil {
				return errRefused
			}
			return nil
		})
		settled <- outcome{
			entryID:  lastPersistedAssistantID(sess, msgs),
			runError: runError,
			errDTO:   errDTO,
			status:   status,
		}
	}()

	select {
	case <-r.Context().Done():
		// The client left. The run continues: the context it was given is
		// detached from this request, and the reservation is released when the
		// run settles.
		return
	case got := <-settled:
		if got.errDTO != nil {
			s.writeJSON(w, got.status, got.errDTO)
			return
		}
		if got.runError != nil {
			// MessageWithParts has no error field and a failed run's assistant
			// message does not say why it failed, so the reason reaches the
			// client as a status instead of a 200 that hides it. A refused
			// reservation is 409 and comes out of here too: it produced no
			// assistant message, and reading the entry first would answer 500
			// "run produced no persisted assistant message" for a request that
			// was never accepted.
			s.writePromptError(w, got.runError)
			return
		}
		if got.entryID == "" {
			s.writeJSON(w, http.StatusInternalServerError, errorDTO{Error: "run produced no persisted assistant message"})
			return
		}
		dto, found := s.assistantMessageDTO(sess, got.entryID)
		if !found {
			s.writeJSON(w, http.StatusInternalServerError, errorDTO{Error: "assistant message is not readable from the transcript"})
			return
		}
		s.writeJSON(w, http.StatusOK, dto)
	}
}

// errRefused ends a PromptContentWait whose before hook declined the request.
// It never reaches a client: the hook records the status to answer instead, and
// the error only tells the handler not to report a run failure.
var errRefused = errors.New("serve: the request was refused before the turn started")

// POST /session/{id}/abort
//
// 200 with a JSON boolean, whether the session was running or idle, matching
// opencode (the client decodes the body and rejects an empty non-JSON reply).
// The cancel is delivered synchronously; the drain is reported by the next
// session.status.
func (s *Server) sessionAbort(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.sessionPreconditions(w, r)
	if !ok {
		return
	}
	sess.Abort()
	s.writeJSON(w, http.StatusOK, true)
}

// sessionPreconditions applies the checks every session-scoped write shares: the
// directory must be the server root, and the session must be one this server
// started. A disk-only session is 404 here because Host.Start cannot adopt an
// existing file, so a prompt to one could never run.
func (s *Server) sessionPreconditions(w http.ResponseWriter, r *http.Request) (*codinghost.Session, bool) {
	if _, errDTO, status := s.directoryParam(r); errDTO != nil {
		s.writeJSON(w, status, errDTO)
		return nil, false
	}
	sess, ok := s.host.Get(r.PathValue("id"))
	if !ok {
		s.writeJSON(w, http.StatusNotFound, errorDTO{Error: "session not found"})
		return nil, false
	}
	return sess, true
}

// decodePromptBody enforces maxBodyBytes and decodes the prompt body. It
// answers the request itself on failure: a truncated body and a malformed one
// are different client bugs, and MaxBytesReader is the only thing in the tree
// that can tell them apart.
func (s *Server) decodePromptBody(w http.ResponseWriter, r *http.Request, body *promptBody) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(body); err != nil {
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			s.writeJSON(w, http.StatusRequestEntityTooLarge, errorDTO{Error: "request body too large"})
			return false
		}
		s.writeJSON(w, http.StatusBadRequest, errorDTO{Error: "invalid body"})
		return false
	}
	return true
}

// bodyContent converts client parts to user content blocks. A file part's data
// URL is split into the mime type and the base64 payload ai.ImageContent
// carries.
func bodyContent(parts []promptPart) ([]ai.UserContentBlock, *errorDTO) {
	content := make([]ai.UserContentBlock, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			content = append(content, ai.TextContent{Text: p.Text})
		case "file":
			content = append(content, ai.ImageContent{MimeType: dataURLMime(p.URL, p.Mime), Data: dataURLPayload(p.URL)})
		}
	}
	if len(content) == 0 {
		return nil, &errorDTO{Error: "message has no text or file part"}
	}
	return content, nil
}

func dataURLMime(url, fallback string) string {
	header, _, ok := strings.Cut(url, ",")
	if !ok || !strings.HasPrefix(header, "data:") {
		return fallback
	}
	return strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
}

func dataURLPayload(url string) string {
	if _, payload, ok := strings.Cut(url, ","); ok && strings.HasPrefix(url, "data:") {
		return payload
	}
	return url
}

// applyModel resolves the client's model reference against the models this
// server offers and swaps the session onto it. SetModel has already stored the
// new model when its audit append fails, so that case answers 500 and is
// recorded rather than hidden.
func (s *Server) applyModel(sess *codinghost.Session, ref modelRef) (*errorDTO, int) {
	models, err := codinghost.AvailableModels(s.services, sess.Inner().Model())
	if err != nil {
		return &errorDTO{Error: err.Error()}, http.StatusInternalServerError
	}
	i := slices.IndexFunc(models, func(candidate *ai.Model) bool {
		return codinghost.ModelProviderID(candidate) == ref.ProviderID && candidate.ID == ref.ModelID
	})
	if i < 0 {
		return &errorDTO{Error: "model not found: " + ref.ProviderID + "/" + ref.ModelID}, http.StatusBadRequest
	}
	if err := sess.Inner().SetModel(models[i]); err != nil {
		return &errorDTO{Error: err.Error()}, http.StatusInternalServerError
	}
	return nil, 0
}

// writePromptError maps a reservation failure to its status.
func (s *Server) writePromptError(w http.ResponseWriter, err error) {
	if errors.Is(err, codinghost.ErrSessionBusy) {
		s.writeJSON(w, http.StatusConflict, errorDTO{Error: "session is busy"})
		return
	}
	s.writeJSON(w, http.StatusInternalServerError, errorDTO{Error: err.Error()})
}

// lastPersistedAssistantID resolves the transcript entry id of the newest
// assistant message the run produced. TurnEndEvent.MessageEntryID never
// reaches Session.Events(), so this is the reconciliation source: the message
// is persisted before its message_end reaches any listener.
func lastPersistedAssistantID(sess *codinghost.Session, msgs []agent.AgentMessage) string {
	entryID := ""
	for _, m := range msgs {
		if m.Assistant == nil {
			continue
		}
		if id, ok := sess.Inner().MessageEntryID(m); ok {
			entryID = id
		}
	}
	return entryID
}

// assistantMessageDTO renders one transcript entry through entriesToMessages,
// so the /message response and GET /session/{id}/message cannot disagree on
// part IDs, the tool-part state upgrade, or the info shape. The slice starts at
// the assistant entry so the toolResult entries that follow still upgrade its
// tool parts, and ends there so nothing later is walked.
func (s *Server) assistantMessageDTO(sess *codinghost.Session, entryID string) (messageDTO, bool) {
	entries := sess.Inner().Entries()
	start := slices.IndexFunc(entries, func(e icodingagent.SessionEntry) bool { return e.Base.ID == entryID })
	if start < 0 {
		return messageDTO{}, false
	}
	root, _ := filepath.EvalSymlinks(s.services.CWD())
	cwd := sess.Inner().CWD()
	if cwd == "" {
		cwd = root
	}
	providerID, modelID, _ := s.sessionModelParts()
	for _, dto := range entriesToMessages(entries[start:], sess.ID(), cwd, root, providerID, modelID) {
		if dto.Info.ID == entryID {
			return dto, true
		}
	}
	return messageDTO{}, false
}
