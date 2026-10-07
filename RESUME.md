# Resume point

**Idle — no wave in progress.** 2026-10-07

- **In flight:** nothing. `serve-allowed-roots-unknown-key-fail-open` (owner option b) shipped
  and merged.
- **Last green slice:** `21d1062` on `wave-serve-allowed-roots-unknown-key-fail-open`, merged
  into `main` (`--no-ff`) as `a0d8365`. Base was `main` at `90861dd`.
- **What shipped:** In `internal/codingagent/settings.go`, `serverKeyIsAllowedRootsNearMiss`
  plus a restructured `checkServerRaw` track a valid `allowedRoots` and the first key on the
  `allow…root` stem that `encoding/json` would not match; a near-miss with no valid
  `allowedRoots` returns an error that rides the existing `GlobalLoadError` →
  `globalServeAllowedRoots` → `failf` path, so `wopr serve` refuses. Padding (whitespace and
  invisible `unicode.Cf` runes such as a pasted U+200B) is treated as a near-miss. Other
  unrecognized `server` keys ride unexported `Settings.serverUnknownKeys` +
  `SettingsManager.GlobalUnknownServerKeys`, and `cmd/wopr/serve.go` prints
  `unrecognized server setting %q ignored` and continues. Tests:
  `TestUnmarshalJSONRejectsNearMissAllowedRootsKey`, `TestLoadWarnsOnUnknownServerKey`,
  `unknown-server-keys-tolerated` subtest, `TestRunServeCommandRefusesOnNearMissAllowedRootsKey`.
  Spec: `docs/plans/2026-10-07-serve-allowed-roots-unknown-key-fail-open.md`.
- **Verification:** spec fan 5 angles (completeness, fidelity, feasibility, leanness,
  runtime-seam) then a 2-angle re-fan, then a 4-angle diff fan (falsifiability, scope,
  runtime-seam, adversarial). Findings fixed: whitespace-padded canonical key still failed open;
  the "warn" half was scoped out and was implemented; warnings were re-routed off `SettingsError`
  (which would mislabel them "Invalid settings file"); a leading zero-width rune bypass was
  closed. Implementer red-proofed the wiring test (guard off → FAIL on the refusal substring).
  Gates green in the worktree and again on merged `main`: `go build ./...`, `go vet ./...`,
  `go test ./cmd/wopr ./internal/codingagent` (251 passed), lint (0 issues),
  `bash -lc 'make check'` all green.
- **Surfaced (READY, in BACKLOG "Follow-ups surfaced by session-directory"):**
  `serve-server-key-near-miss-fail-open` — an outer `server` key typo/paddding drops the whole
  server object and still fails open (pre-existing, same class, one level up);
  `serve-unknown-server-key-schema-drift` (low) — `checkServerRaw`'s known-key set duplicates
  `ServerSettings`' JSON tags.
- **Accepted residuals (spec Open Questions):** JSON `null` for a near-miss or `allowedRoots` is
  treated as absent; explicit empty `allowedRoots: []` beside a near-miss does not fail (empty is
  the documented unrestricted value); typos with no shared stem (`alowedRoots`, `allowedRots`,
  `roots`, `allowed`) are not caught.
- **Tracker:** all session-directory items DONE except the two new READY follow-ups above.
- **tmux PATH nuance:** `tmux` lives on PATH only for login shells (`bash -lc`); run `make check`
  via `bash -lc` or it can fail at `test-prereqs`.
- **Remote:** `main` is ahead of `origin/main`; nothing has been pushed.

This file lives at the primary checkout on purpose. `wave` runs inside
`.worktrees/wave-<id>/`, which merge prunes; a resume point written inside a
worktree dies with it. Write it here, at `/home/kc/dev/wopr/RESUME.md`, from any
worktree.
