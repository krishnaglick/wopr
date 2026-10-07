package serve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alexrudloff/wopr/agent"
	"github.com/alexrudloff/wopr/ai"
	codinghost "github.com/alexrudloff/wopr/coding/host"
	"github.com/alexrudloff/wopr/coding/version"
	"github.com/alexrudloff/wopr/internal/codingagent/askuser"
	"github.com/alexrudloff/wopr/internal/codingagent/tools"
)

// The opencode SSE event names this server emits. Event.Type IS the name: the
// bus carries it from the producer to the frame writer, so no layer translates
// between the agent's events and the client's vocabulary.
//
// Twelve of the seventeen names the client switches on have a live trigger in
// wopr. The other five have none, and none is invented for them:
// permission.asked and permission.replied need an approval policy and a
// blocking hook, which do not exist and are not designed (serve-7);
// session.deleted needs a session deletion, and DELETE /session/{id} is a 501
// stub; message.removed and message.part.removed need a removal, and nothing in
// wopr removes either — a failed tool is state.status = "error" on a part that
// stays. Emitting a placeholder for one would render an empty permission prompt
// or a phantom deletion the client cannot reconcile.
const (
	eventServerConnected    = "server.connected"
	eventServerHeartbeat    = "server.heartbeat"
	eventMessageUpdated     = "message.updated"
	eventMessagePartUpdated = "message.part.updated"
	eventMessagePartDelta   = "message.part.delta"
	eventSessionUpdated     = "session.updated"
	eventSessionError       = "session.error"
	eventSessionStatus      = "session.status"
	eventSessionIdle        = "session.idle"
	eventTodoUpdated        = "todo.updated"
	eventQuestionAsked      = "question.asked"
	eventQuestionReplied    = "question.replied"
	eventQuestionRejected   = "question.rejected"
)

const (
	// maxSSEConnections caps concurrent /global/event streams. The next one
	// answers 503 rather than taking another sseQueueBytes of resident memory.
	maxSSEConnections = 16
	// sseQueueBytes is the per-connection queued-byte budget: 2 × maxTailBytes.
	// The session tail-read window is the largest thing a client can ask for in
	// one response, so a budget of twice it holds that plus a second frame with
	// room to spare — and it holds wopr's largest pinned entry, 17 MiB.
	sseQueueBytes = 2 * maxTailBytes
	// sseHeartbeat both keeps an otherwise idle stream alive through mobile and
	// proxy idle timeouts and is the Next timeout that paces the write loop.
	sseHeartbeat = 15 * time.Second
	// sseDropError is the error name on the terminal byte-bound frame.
	sseDropError = "SSEDropped"
)

// ─── Payloads ──────────────────────────────────────────────────────────────

// sseConnected answers server.connected.
type sseConnected struct {
	SessionID string `json:"sessionID"`
	Version   string `json:"version"`
}

// sseHeartbeatFrame answers server.heartbeat.
type sseHeartbeatFrame struct {
	SessionID string `json:"sessionID"`
}

// sseMessageUpdated answers message.updated. info carries the four fields the
// client requires of a Message; the parts arrive as message.part.updated keyed
// by partID.
type sseMessageUpdated struct {
	SessionID string         `json:"sessionID"`
	Info      messageInfoDTO `json:"info"`
}

// ssePartUpdated answers message.part.updated.
type ssePartUpdated struct {
	SessionID string  `json:"sessionID"`
	Part      partDTO `json:"part"`
	Time      int64   `json:"time"`
}

// ssePartDelta answers message.part.delta. field and partID are required by the
// client's decoder and are what let it route a delta to the right part.
type ssePartDelta struct {
	SessionID string `json:"sessionID"`
	MessageID string `json:"messageID"`
	PartID    string `json:"partID"`
	Field     string `json:"field"`
	Delta     string `json:"delta"`
}

// sseError is the nested error object of session.error.
type sseError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// sseSessionError answers session.error, including the terminal byte-bound
// frame, which is the only [DONE] this stream emits deliberately.
type sseSessionError struct {
	SessionID string   `json:"sessionID"`
	Error     sseError `json:"error"`
}

// sseSessionUpdated answers session.updated.
type sseSessionUpdated struct {
	SessionID string     `json:"sessionID"`
	Info      sessionDTO `json:"info"`
}

// sseSessionStatus answers session.status. opencode nests the state under
// status: {"sessionID":…,"status":{"type":"busy"}}; a top-level type is a
// different shape the client does not read.
type sseSessionStatus struct {
	SessionID string               `json:"sessionID"`
	Status    sseSessionStatusType `json:"status"`
}

type sseSessionStatusType struct {
	Type string `json:"type"`
}

// sseSessionIdle answers session.idle, the turn-complete marker the client
// waits for to settle a run (wopr previously emitted only session.status).
type sseSessionIdle struct {
	SessionID string `json:"sessionID"`
}

// sseTodoUpdated answers todo.updated.
type sseTodoUpdated struct {
	SessionID string       `json:"sessionID"`
	Todos     []tools.Todo `json:"todos"`
}

// sseQuestion answers question.asked, question.replied and question.rejected.
// The four question fields are the entry GET /question serves, so a card built
// from the stream and one built from a poll cannot disagree; answer is nil on
// the question and carries the choice, the free text, or the decline after it.
type sseQuestion struct {
	questionDTO
	Answer *askuser.Answer `json:"answer,omitempty"`
}

// ─── Handler ───────────────────────────────────────────────────────────────

// GET /global/event
//
// One stream for every session. It ends three ways and only three: the client
// disconnects, the connection exceeds sseQueueBytes and receives the terminal
// drop frame, or the server stops — which is not implemented, because
// cmd/wopr/serve.go has no shutdown path.
func (s *Server) globalEvent(w http.ResponseWriter, r *http.Request) {
	if live := s.sseConnections.Add(1); live > maxSSEConnections {
		s.sseConnections.Add(-1)
		s.writeJSON(w, http.StatusServiceUnavailable, errorDTO{Error: "too many event stream connections"})
		return
	}
	defer s.sseConnections.Add(-1)

	// One subscription per connection, released on every return path below: a
	// leaked handle is a queue every Publish still fans out to, plus its bytes.
	sub := s.host.Subscribe(sseQueueBytes)
	defer sub.Close()

	// The client sends Accept: text/event-stream and Cache-Control: no-cache;
	// all four headers are answered. X-Accel-Buffering keeps an intermediary
	// from holding frames until the stream ends.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// server.connected is the first frame: the client treats it as proof the
	// stream is live, and it is the one answer a per-session stream cannot give.
	connected, err := json.Marshal(sseConnected{SessionID: "", Version: version.Version})
	if err != nil {
		return
	}
	if err := sseWrite(w, eventServerConnected, connected); err != nil {
		return
	}
	if !sseFlush(w) {
		return
	}

	for {
		name, payload, terminal, err := sub.Next(r.Context(), sseHeartbeat)
		if err != nil {
			// The heartbeat interval elapsed with nothing queued. The client
			// disconnected or the subscription closed otherwise.
			if !errors.Is(err, context.DeadlineExceeded) {
				return
			}
			beat, err := json.Marshal(sseHeartbeatFrame{SessionID: ""})
			if err != nil {
				return
			}
			if err := sseWrite(w, eventServerHeartbeat, beat); err != nil {
				return
			}
			if !sseFlush(w) {
				return
			}
			continue
		}
		if terminal {
			// The byte bound. The queued prefix was delivered above, so the
			// client holds a coherent message and a marker of where it stopped.
			drop, err := json.Marshal(sseSessionError{SessionID: "", Error: sseError{
				Name:    sseDropError,
				Message: "connection exceeded its " + strconv.Itoa(sseQueueBytes>>20) + "MiB buffer",
			}})
			if err != nil {
				return
			}
			if err := sseWrite(w, eventSessionError, drop); err != nil {
				return
			}
			if !sseDone(w) {
				return
			}
			return
		}
		if err := sseWrite(w, name, payload); err != nil {
			return
		}
		if !sseFlush(w) {
			return
		}
	}
}

// sseEnvelope is opencode's SSE frame: a wrapped payload carrying the event id,
// type and properties. The client reads data.payload.type and
// data.payload.properties; a bare data object (wopr's old frame) leaves
// data.payload undefined and throws on the first frame.
type sseEnvelope struct {
	Payload sseEnvelopePayload `json:"payload"`
}

type sseEnvelopePayload struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
}

// newEventID mints an opencode-shaped envelope id. The client only needs it
// unique; it is never echoed back.
func newEventID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "evt_" + strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return "evt_" + hex.EncodeToString(b[:])
}

// sseWrite writes one SSE frame in opencode's envelope. A write error is the
// only signal that the connection is gone, so it ends the handler.
func sseWrite(w http.ResponseWriter, name string, payload []byte) error {
	frame, err := json.Marshal(sseEnvelope{Payload: sseEnvelopePayload{
		ID:         newEventID(),
		Type:       name,
		Properties: payload,
	}})
	if err != nil {
		return err
	}
	frame = append(frame, '\n', '\n')
	return writeAll(w, append([]byte("data: "), frame...))
}

// writeAll writes b in full, since a single Write may be short.
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

// sseFlush pushes buffered frames to the client. Without it the response is
// held by net/http until the handler returns, which for a stream is never.
func sseFlush(w http.ResponseWriter) bool {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}
	flusher.Flush()
	return true
}

// sseDone writes the [DONE] sentinel and flushes it, closing the stream. It is
// only ever written on the byte-bound drop: this stream is shared by every
// session, so a per-turn sentinel would make the client tear it down and
// reconnect after every turn, and there is no Last-Event-ID replay to resume
// from.
func sseDone(w http.ResponseWriter) bool {
	if _, err := w.Write([]byte("data: [DONE]\n\n")); err != nil {
		return false
	}
	return sseFlush(w)
}

// ─── The agent → opencode mapper ───────────────────────────────────────────

// eventMapper turns one session's agent events into opencode SSE frames.
//
// It holds the per-session provisional message counter, so it needs no lock: a
// session's forwarder is a single goroutine and calls Translate only from it.
type eventMapper struct {
	sessionID string
	// cwd is the session's own working directory, or the resolved server root
	// when the session has none. root is always the resolved server root.
	cwd  string
	root string
	// providerID/modelID are the server spec fallback for a streamed assistant
	// message that omits its own provider/model.
	providerID string
	modelID    string
	// next is the number of messages that have started in this session's life.
	next int
	// current is the index of the message being streamed, set at its
	// MessageStartEvent. Every message emits exactly one start before its
	// updates and its end, so it is stable for the whole message.
	current int
}

// newEventMapper returns the Translator the host runs for one session. It is a
// *Server method because the mapper carries the server's resolved root and
// model spec alongside the session's own cwd, read once at construction.
func (s *Server) newEventMapper(sessionID string) codinghost.Translator {
	root, _ := filepath.EvalSymlinks(s.services.CWD())
	cwd := root
	if ls, ok := s.host.Get(sessionID); ok {
		if got := ls.Inner().CWD(); got != "" {
			cwd = got
		}
	}
	providerID, modelID, _ := s.sessionModelParts()
	mapper := &eventMapper{
		sessionID:  sessionID,
		cwd:        cwd,
		root:       root,
		providerID: providerID,
		modelID:    modelID,
	}
	return mapper.translate
}

// translate maps one agent event onto zero or more bus events. Zero events is
// the common case: an event the mapper does not represent produces nothing
// rather than an invented client event.
func (m *eventMapper) translate(ev agent.AgentEvent, entryID string) ([]codinghost.Event, error) {
	switch e := ev.(type) {
	case agent.MessageStartEvent:
		m.current = m.next
		m.next++
		return nil, nil
	case agent.MessageUpdateEvent:
		return m.messageUpdate(e)
	case agent.MessageEndEvent:
		return m.messageEnd(e, entryID)
	case agent.AgentStartEvent:
		return m.one(m.status("busy")), nil
	case agent.AgentSettledEvent:
		return []codinghost.Event{m.status("idle"), m.idle()}, nil
	case agent.AutoRetryStartEvent:
		return append(m.one(m.status("retry")), m.error("AutoRetry", e.ErrorMessage)), nil
	case agent.AutoRetryEndEvent:
		return m.one(m.status("idle")), nil
	case agent.CompactionEndEvent:
		if e.ErrorMessage == "" {
			return nil, nil
		}
		return m.one(m.error("Compaction", e.ErrorMessage)), nil
	case agent.ToolExecutionStartEvent:
		return m.toolStart(e), nil
	case agent.ToolExecutionUpdateEvent:
		return m.one(m.toolSnapshot(e.ToolCallID, e.ToolName, "running", e.Content, nil)), nil
	case agent.ToolExecutionEndEvent:
		status := "completed"
		if e.Result.IsError {
			status = "error"
		}
		events := m.one(m.toolSnapshot(e.ToolCallID, e.ToolName, status, e.Result.Content, e.Result.Details))
		// todo.updated rides the end of a todowrite call, which is the only
		// event that changes the list. A rejected todowrite carries nil details
		// and changes nothing, so it must not broadcast an empty list.
		if todos := todosFromDetails(e.ToolName, e.Result.Details); todos != nil {
			events = append(events, m.todoUpdated(m.sessionID, todos))
		}
		return events, nil
	}
	return nil, nil
}

// messageUpdate maps a streamed assistant message event. The role filter is
// mandatory: message_end also fires for user and toolResult messages, and
// toolResult is not a role opencode's Message can decode, so a mapper without
// it emits a bubble the client cannot render.
func (m *eventMapper) messageUpdate(ev agent.MessageUpdateEvent) ([]codinghost.Event, error) {
	if ev.Message.Assistant == nil {
		return nil, nil
	}
	messageID := m.provisionalID()
	switch stream := ev.AssistantMessageEvent.(type) {
	// Three true deltas. A tool-call delta is an arguments delta: routing it to
	// message.part.updated instead would emit a tool part with empty arguments.
	case ai.TextDeltaEvent:
		return m.one(m.delta(messageID, partID(messageID, "text", stream.ContentIndex, ""), "text", stream.Delta)), nil
	case ai.ThinkingDeltaEvent:
		return m.one(m.delta(messageID, partID(messageID, "reasoning", stream.ContentIndex, ""), "reasoning", stream.Delta)), nil
	case ai.ToolCallDeltaEvent:
		// The index is over the streamed message's content, so the call id has
		// to come from there: a tool-call delta carries no id of its own.
		// An OpenAI-compatible server can stream tool_calls with no id, so the
		// call id can be empty. toolPart and the transcript build their tool id
		// from the same empty value, so the empty form is what everything else
		// emits too — falling back keeps the delta agreeing with the part it
		// completes, and the arguments still stream. Dropping it instead left
		// the live tool bubble empty until the client refetched history.
		callID := toolCallID(stream.Partial, stream.ContentIndex)
		return m.one(m.delta(messageID, partID(messageID, "tool", stream.ContentIndex, callID), "arguments", stream.Delta)), nil
	case ai.TextEndEvent:
		return m.one(m.part(textPart(messageID, m.sessionID, stream.ContentIndex, stream.Content))), nil
	case ai.ThinkingEndEvent:
		return m.one(m.part(reasoningPart(messageID, m.sessionID, stream.ContentIndex, stream.Content))), nil
	case ai.ToolCallEndEvent:
		return m.one(m.part(toolPart(m.sessionID, messageID, stream.ContentIndex, stream.ToolCall.ID, stream.ToolCall.Name))), nil
	}
	return nil, nil
}

// messageEnd is the reconciliation burst. Deltas streamed under a provisional
// id, because wopr allocates the transcript entry at message_end and no
// authoritative id exists while a message streams. Here the entry is on disk,
// so the message is re-announced under its real id and its parts are published
// with the ids the transcript will report — which is what makes the two agree
// by construction rather than by luck.
//
// message.updated is emitted for assistant messages only. A user or toolResult
// end produces nothing: the app already holds the user message it sent.
func (m *eventMapper) messageEnd(ev agent.MessageEndEvent, entryID string) ([]codinghost.Event, error) {
	if ev.Message.Assistant == nil || entryID == "" {
		return nil, nil
	}
	now := time.Now().UnixMilli()
	blocks := ev.Message.Assistant.Content
	events := []codinghost.Event{m.event(eventMessageUpdated, sseMessageUpdated{
		SessionID: m.sessionID,
		Info: messageInfo(m.sessionID, m.cwd, m.root, entryID, "assistant", "", now,
			ev.Message.Assistant, m.providerID, m.modelID),
	})}
	// The index runs over the full content slice, enumerated before any block is
	// filtered out, so it equals ai.*Event.ContentIndex even where a block yields
	// no part (ai.ServerToolContent) — that is the drift partID cannot prevent.
	for i, block := range blocks {
		var part partDTO
		switch b := block.(type) {
		case ai.TextContent:
			part = textPart(entryID, m.sessionID, i, b.Text)
		case ai.ThinkingContent:
			part = reasoningPart(entryID, m.sessionID, i, b.Thinking)
		case ai.ToolCall:
			part = toolPart(m.sessionID, entryID, i, b.ID, b.Name)
		default:
			continue
		}
		events = append(events, m.part(part))
	}
	if ev.Message.Assistant.ErrorMessage != "" {
		// Without this a run that streamed deltas and then failed leaves the app
		// with a blank bubble and a session that stays busy.
		events = append(events, m.error("AssistantError", ev.Message.Assistant.ErrorMessage))
	}
	return events, nil
}

// toolStart opens a tool part. The event carries no content index, and the tool
// form of partID is identified by its call id, not its index, so it is the same
// part the content-indexed events and the transcript name.
func (m *eventMapper) toolStart(ev agent.ToolExecutionStartEvent) []codinghost.Event {
	part := toolPart(m.sessionID, m.provisionalID(), 0, ev.ToolCallID, ev.ToolName)
	part.State = json.RawMessage(`{"status":"running"}`)
	return m.one(m.part(part))
}

// toolSnapshot republishes a tool part with its complete-so-far output.
// ToolExecutionUpdateEvent is a snapshot, never a delta, so it is a
// message.part.updated and never a message.part.delta.
func (m *eventMapper) toolSnapshot(callID, name, status, output string, details any) codinghost.Event {
	part := toolPart(m.sessionID, m.provisionalID(), 0, callID, name)
	state := map[string]any{"status": status, "output": output}
	if todos := todosFromDetails(name, details); len(todos) > 0 {
		state["metadata"] = map[string]any{"todos": todos}
	}
	stateBytes, _ := json.Marshal(state)
	part.State = stateBytes
	return m.part(part)
}

func (m *eventMapper) todoUpdated(sessionID string, todos []tools.Todo) codinghost.Event {
	if todos == nil {
		todos = []tools.Todo{}
	}
	return m.event(eventTodoUpdated, sseTodoUpdated{SessionID: sessionID, Todos: todos})
}

// todosFromDetails decodes the todos a tool result carries. Details is the
// tool's own value on the live path and map[string]any after a JSON round trip,
// so the decode goes through JSON rather than a type assertion; a result from
// any other tool decodes to nothing.
func todosFromDetails(tool string, details any) []tools.Todo {
	if tool != tools.Name || details == nil {
		return nil
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return nil
	}
	var todos []tools.Todo
	if err := json.Unmarshal(raw, &todos); err != nil {
		return nil
	}
	return todos
}

// provisionalID is the streamed id of the message in flight.
func (m *eventMapper) provisionalID() string {
	return "msg_" + m.sessionID + "-" + strconv.Itoa(m.current)
}

func (m *eventMapper) delta(messageID, id, field, text string) codinghost.Event {
	return m.event(eventMessagePartDelta, ssePartDelta{
		SessionID: m.sessionID,
		MessageID: messageID,
		PartID:    id,
		Field:     field,
		Delta:     text,
	})
}

func (m *eventMapper) part(p partDTO) codinghost.Event {
	return m.event(eventMessagePartUpdated, ssePartUpdated{SessionID: m.sessionID, Part: p, Time: time.Now().UnixMilli()})
}

func (m *eventMapper) status(kind string) codinghost.Event {
	return m.event(eventSessionStatus, sseSessionStatus{SessionID: m.sessionID, Status: sseSessionStatusType{Type: kind}})
}

func (m *eventMapper) idle() codinghost.Event {
	return m.event(eventSessionIdle, sseSessionIdle{SessionID: m.sessionID})
}

func (m *eventMapper) error(name, message string) codinghost.Event {
	return m.event(eventSessionError, sseSessionError{SessionID: m.sessionID, Error: sseError{Name: name, Message: message}})
}

func (m *eventMapper) event(name string, data any) codinghost.Event {
	return codinghost.Event{Type: name, Session: m.sessionID, Data: data}
}

func (m *eventMapper) one(ev codinghost.Event) []codinghost.Event {
	return []codinghost.Event{ev}
}

// ─── Part construction ─────────────────────────────────────────────────────

// textPart, reasoningPart and toolPart build the partDTO the stream and the
// transcript both emit, so the two cannot disagree on identity or shape. The
// index is over the message's full content slice on both sides.
func textPart(messageID, sessionID string, index int, text string) partDTO {
	return partDTO{
		ID:        partID(messageID, "text", index, ""),
		MessageID: messageID,
		SessionID: sessionID,
		Type:      "text",
		Text:      text,
	}
}

func reasoningPart(messageID, sessionID string, index int, thinking string) partDTO {
	return partDTO{
		ID:        partID(messageID, "reasoning", index, ""),
		MessageID: messageID,
		SessionID: sessionID,
		Type:      "reasoning",
		Text:      thinking,
	}
}

func toolPart(sessionID, messageID string, index int, callID, name string) partDTO {
	return partDTO{
		ID:        partID(messageID, "tool", index, callID),
		MessageID: messageID,
		SessionID: sessionID,
		Type:      "tool",
		Tool:      name,
		CallID:    callID,
		// ponytail: pending is the transcript's tool-call state and the client
		// treats every inner state field as optional. The stream's running
		// state arrives separately from ToolExecutionStartEvent; unifying them
		// would mean the transcript inventing a state it never observed.
		State: json.RawMessage(`{"status":"pending"}`),
	}
}

// toolCallID resolves the call id of the tool call at contentIndex, or "" when
// there is none to resolve: no streamed message, an index past what it carries,
// or a block that is not a tool call. Empty is a real answer, not an error —
// a provider can send a tool-call delta before it names the call.
func toolCallID(partial *ai.AssistantMessage, contentIndex int) string {
	if partial == nil || contentIndex < 0 || contentIndex >= len(partial.Content) {
		return ""
	}
	call, ok := partial.Content[contentIndex].(ai.ToolCall)
	if !ok {
		return ""
	}
	return call.ID
}
