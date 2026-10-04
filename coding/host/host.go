package host

import (
	"context"
	"errors"
	"sync"

	"github.com/alexrudloff/wopr/coding"
)

var ErrSessionBusy = errors.New("host: session already has a run in flight")

// Host owns every session a served process runs.
type Host struct {
	svcs     *coding.Services
	bus      *Bus
	registry map[string]*Session
	mu       sync.RWMutex
}

// New returns a Host over svcs. sink, when non-nil, also receives every published
// event's bytes: the single-consumer path, named by the item
// (/home/kc/dev/wopr/BACKLOG.md:39-40). Nil in slice 1; serve-2's rpc writer is
// the first real sink.
func New(svcs *coding.Services, sink func(Event, []byte)) *Host {
	h := &Host{
		svcs:     svcs,
		bus:      newBus(sink),
		registry: make(map[string]*Session),
	}
	return h
}

// StartOptions is what a caller outside coding/host may configure. The model is
// a "provider/model" spec because the host resolves it: no caller here can import
// coding to build an *ai.Model itself.
type StartOptions struct {
	Model string // required, "provider/model"
}

// Start creates one session and registers it under its id. A duplicate id
// closes the new session and returns an error.
func (h *Host) Start(opts StartOptions) (*Session, error) {
	if opts.Model == "" {
		return nil, errors.New("host: StartOptions.Model is required")
	}
	model, err := coding.BuildModel(opts.Model, h.svcs)
	if err != nil {
		return nil, err
	}
	sess, err := coding.StartSession(h.svcs, coding.SessionStartOptions{
		Model: model,
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
}

func (s *Session) ID() string {
	return s.id
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

// Close deregisters the session before closing, so a closed session cannot be
// fetched from the registry and the map does not accumulate dead entries.
func (s *Session) Close() error {
	s.host.mu.Lock()
	delete(s.host.registry, s.id)
	s.host.mu.Unlock()
	return s.sess.Close()
}
