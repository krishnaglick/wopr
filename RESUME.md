# Resume point

**Idle — no wave in progress.** 2026-10-07

- **In flight:** nothing. `serve-server-key-near-miss-fail-open` shipped and merged.
- **Last green slice:** `9e8b9ed` on `wave-serve-server-key-near-miss-fail-open`, merged into
  `main` (`--no-ff`) as `bce5333`. Base was `main` at `1f4d95e`.
- **What shipped:** In `internal/codingagent/settings.go`, new `serverKeyIsServerNearMiss` — it
  folds ASCII case, discards every non-ASCII rune (any category, including case-folding letters
  such as KELVIN SIGN), and flags a top-level key whose normalized form CONTAINS the `server`
  stem. `checkServerRaw` tracks `validRootsSeen`/`serverNearMiss`: a near-miss outer key with no
  valid exact `server.allowedRoots` returns `"<key>" is not a known key; did you mean server?`,
  which rides the existing `GlobalLoadError` → `globalServeAllowedRoots` → `failf` path, so
  `wopr serve` refuses. A valid exact `server` wins and the near-miss key rides the existing
  `serverUnknownKeys` warning. A near-miss holding only `port` also refuses (deliberate
  fail-closed, owner option-b precedent). Tests: `TestUnmarshalJSONRejectsNearMissServerKey`,
  extended `TestLoadWarnsOnUnknownServerKey`, `near-miss-outer-key` +
  `unrelated-top-level-key-tolerated` subtests, `TestRunServeCommandRefusesOnNearMissServerKey`.
  Spec: `docs/plans/2026-10-07-serve-server-key-near-miss-fail-open.md`.
- **Verification:** spec fan 5 angles, then three correction rounds driven by diff fans; the
  adversarial security angle escalated and was closed in three steps: interior padding (not just
  leading/trailing), invisible runes outside a drop-list category (U+2800, Hangul fillers, Mc) →
  keep-only-ASCII rule, and non-ASCII case-folding runes (KELVIN U+212A, İ U+0130) → manual ASCII
  fold before dropping. Final 3-angle fan (security, tolerance, test-evidence) clean; the only
  remaining security findings are the documented residuals below. Red-proofs show every fixture is
  load-bearing. Gates green in the worktree and again on merged `main`: `go build ./...`,
  `go vet ./...`, `go test ./cmd/wopr ./internal/codingagent` (255 passed), lint (0 issues),
  `bash -lc 'make check'` all green.
- **Accepted residuals (spec Open Questions):** name mutations that break the contiguous `server`
  stem — ASCII insertions inside it (`serxver`), deletions/transpositions (`srvr`, `serve`,
  `srever`) — and non-ASCII letter confusables (`ｓｅｒｖｅｒ`, Cyrillic `ѕerver`) still fail open;
  catching them needs edit distance, which the shipped inner-key item also rejects. A near-miss
  outer key holding only `port` refuses.
- **Surfaced:** none new. `serve-unknown-server-key-schema-drift` (low) remains READY in BACKLOG.
- **Tracker:** `serve-server-key-near-miss-fail-open` is DONE; the schema-drift item is the only
  READY follow-up from the serve fail-open batch.
- **tmux PATH nuance:** `tmux` lives on PATH only for login shells (`bash -lc`); run `make check`
  via `bash -lc` or it can fail at `test-prereqs`.
- **Remote:** `main` is ahead of `origin/main`; nothing has been pushed.

This file lives at the primary checkout on purpose. `wave` runs inside
`.worktrees/wave-<id>/`, which merge prunes; a resume point written inside a
worktree dies with it. Write it here, at `/home/kc/dev/wopr/RESUME.md`, from any
worktree.
