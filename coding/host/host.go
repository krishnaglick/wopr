package host

import (
	"context"
	"errors"
	"sync"

	"github.com/alexrudloff/wopr/agent"
	"github.com/alexrudloff/wopr/ai"
	"github.com/alexrudloff/wopr/coding"
)

// Services is the coding.Services type, re-exported so serve can use it
// without importing coding directly.
type Services = coding.Services

// ErrSessionBusy is returned when a session already has a run in flight.
var ErrSessionBusy = errors.New("host: session already has a run in flight")

// ErrNoModel is returned when StartOptions.Model is not set.
var ErrNoModel = errors.New("host: StartOptions.Model is required")

// Host owns every session a served process runs.
type Host struct {
	svcs     *coding.Services
	bus      *Bus
	registry map[string]*Session
	mu       sync.RWMutex
}

// New returns a Host over svcs. sink, when non-nil, also receives every published
// event's bytes: the single-consumer path, named by the item
// (/home/kc/dev/wopr/BACKLOG.md:39-40). ServeRPC takes *coding.Services directly
// and writes its own JSON lines, so the bus is not on the RPC path.
func New(svcs *coding.Services, sink func(Event, []byte)) *Host {
	h := &Host{
		svcs:     svcs,
		bus:      newBus(sink),
		registry: make(map[string]*Session),
	}
	return h
}

// Services returns the coding.Services for callers that need it.
func (h *Host) Services() *coding.Services {
	return h.svcs
}

// Sessions returns a snapshot of the registry under h.mu.RLock.
func (h *Host) Sessions() []*Session {
	h.mu.RLock()
	defer h.mu.RUnlock()
	sessions := make([]*Session, 0, len(h.registry))
	for _, s := range h.registry {
		sessions = append(sessions, s)
	}
	return sessions
}

// StartOptions is what a caller outside coding/host may configure. The model is
// a "provider/model" spec because the host resolves it: no caller here can import
// coding to build an *ai.Model itself.
type StartOptions struct {
	Model      string // required, "provider/model"
	SessionDir string // optional, empty means default dir
}

// Start creates one session and registers it under its id. A duplicate id
// closes the new session and returns an error.
func (h *Host) Start(opts StartOptions) (*Session, error) {
	if opts.Model == "" {
		return nil, ErrNoModel
	}
	model, err := coding.BuildModel(opts.Model, h.svcs)
	if err != nil {
		return nil, err
	}
	sess, err := coding.StartSession(h.svcs, coding.SessionStartOptions{
		Model:      model,
		SessionDir: opts.SessionDir,
	})
	if err != nil {
		return nil, err
	}
	id := sess.ID()
	wrapped := &Session{
		host: h,
		id:   id,
		sess: sess,
	}
	h.mu.Lock()
	if _, exists := h.registry[id]; exists {
		h.mu.Unlock()
		_ = sess.Close() // close error dropped; duplicate-id error takes precedence
		return nil, errors.New("host: session id already registered")
	}
	h.registry[id] = wrapped
	h.mu.Unlock()
	return wrapped, nil
}

// Get returns a registered session by id, or nil if not found.
func (h *Host) Get(id string) (*Session, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.registry[id]
	return s, ok
}

// Session is one session the host owns. It does not embed *coding.Session: an
// exported field would let a caller reach Send and bypass the run rule below.
type Session struct {
	host *Host
	id   string
	sess *coding.Session
	run  sync.Mutex

	// cancel aborts the run this Session owns, from the reservation until the
	// run settles. RequestAbort only reaches a run whose cancel coding has
	// already stored, which happens after beginAgentRun — inside sendContent,
	// not at the reservation.
	cancelMu sync.Mutex
	cancel   context.CancelFunc
}

func (s *Session) ID() string {
	return s.id
}

// Inner returns the underlying *coding.Session.
// This is the deliberate, documented escape hatch for the read surface;
// serve never calls Send on it.
func (s *Session) Inner() *coding.Session {
	return s.sess
}

// Prompt sends a prompt to the session. It returns ErrSessionBusy if the
// session already has a run in flight.
func (s *Session) Prompt(ctx context.Context, text string) error {
	if !s.run.TryLock() {
		return ErrSessionBusy
	}
	defer s.run.Unlock()
	_, err := s.sess.Send(ctx, text)
	return err
}

// PromptContent reserves the session's single run slot and starts the run on a
// goroutine it owns, returning as soon as the reservation lands. It returns
// ErrSessionBusy synchronously when the slot is taken. Everything after the
// reservation is reported on the event bus.
//
// ctx is detached inside this method: an accepted run outlives its caller, and
// an http.Request.Context is cancelled the moment the handler returns, which
// would kill the run this just accepted while the caller still answered 204.
func (s *Session) PromptContent(ctx context.Context, content []ai.UserContentBlock) error {
	if !s.run.TryLock() {
		return ErrSessionBusy
	}
	runCtx, cancel := s.ownRun(ctx)
	go func() {
		defer s.releaseRun(cancel)
		_, _ = s.sess.SendContentWithPreflight(runCtx, content, nil)
	}()
	return nil
}

// PromptContentWait reserves the same single run slot and blocks until the run
// settles, returning the messages it produced. It returns ErrSessionBusy when
// the slot is taken. ctx is detached for the same reason as PromptContent's;
// the caller abandons the wait on its own cancellation.
func (s *Session) PromptContentWait(ctx context.Context, content []ai.UserContentBlock) ([]agent.AgentMessage, error) {
	if !s.run.TryLock() {
		return nil, ErrSessionBusy
	}
	runCtx, cancel := s.ownRun(ctx)
	defer s.releaseRun(cancel)
	return s.sess.SendContentWithPreflight(runCtx, content, nil)
}

// ownRun detaches ctx from its cancellation, makes it cancellable, and stores
// the cancel for Abort. A cancel delivered before beginAgentRun still stops the
// run, because context.WithCancel over an already-cancelled parent yields an
// already-cancelled child.
func (s *Session) ownRun(ctx context.Context) (context.Context, context.CancelFunc) {
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.cancelMu.Lock()
	s.cancel = cancel
	s.cancelMu.Unlock()
	return runCtx, cancel
}

// releaseRun clears the stored cancel and returns the run slot.
func (s *Session) releaseRun(cancel context.CancelFunc) {
	s.cancelMu.Lock()
	s.cancel = nil
	s.cancelMu.Unlock()
	cancel()
	s.run.Unlock()
}

// Abort cancels the run this Session owns when the reservation is still
// outstanding, then asks the session to abort whatever run it has. It does not
// wait for the drain: the cancel has already been delivered, and a later
// session.status reports idle. Aborting an idle session is success.
func (s *Session) Abort() {
	s.cancelMu.Lock()
	cancel := s.cancel
	s.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.sess.RequestAbort()
}

// Close deregisters the session before closing, so a closed session cannot be
// fetched from the registry and the map does not accumulate dead entries.
func (s *Session) Close() error {
	s.host.mu.Lock()
	delete(s.host.registry, s.id)
	s.host.mu.Unlock()
	return s.sess.Close()
}
