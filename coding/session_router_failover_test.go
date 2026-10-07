package coding

import (
	"context"
	"testing"

	"github.com/alexrudloff/wopr/agent"
)

// TestSessionFailoverAdvancesToTheNextRoutedModel drives a routed run whose
// first model fails with a quota error and whose second answers: the run must
// advance to the next routed model (test-faux/faux-2, Fallback true) rather
// than retrying the failed one, and the failed attempt must be omitted from
// context.
func TestSessionFailoverAdvancesToTheNextRoutedModel(t *testing.T) {
	t.Setenv("WOPR_TEST_FAUX", "1")
	h := newRecoveryHarness(t, harnessOptions{router: `{
  "enabled": true,
  "tiers": [
    {"name": "test", "cost": "free-local", "capability": 0.5,
     "models": [
       {"provider": "test-faux", "model": "faux-1"},
       {"provider": "test-faux", "model": "faux-2"}
     ]}
  ]
}`})

	if _, err := h.session.Send(context.Background(), "Trigger: free tier limit"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	events := h.settle(t)

	firstIndex, fallbackIndex := -1, -1
	for i, event := range events {
		route, ok := event.(agent.RouteEvent)
		if !ok {
			continue
		}
		switch {
		case route.Model == "faux-1" && !route.Fallback && firstIndex < 0:
			firstIndex = i
		case route.Model == "faux-2" && route.Fallback && fallbackIndex < 0:
			fallbackIndex = i
		}
	}
	if firstIndex < 0 {
		t.Fatal("no RouteEvent for the first routed model test-faux/faux-1")
	}
	if fallbackIndex < 0 {
		t.Fatal("no fallback RouteEvent for test-faux/faux-2")
	}
	if fallbackIndex <= firstIndex {
		t.Fatalf("fallback route (index %d) did not follow the first route (index %d)", fallbackIndex, firstIndex)
	}
	if !projectionContains(h, "failover-ok") {
		t.Fatal("projection does not contain the second attempt's text failover-ok")
	}
	if len(h.entries("context_edit")) < 1 {
		t.Fatal("the failed attempt was not omitted from context")
	}
}
