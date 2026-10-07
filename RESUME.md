# Resume point

**Idle — no wave in progress.** 2026-10-07

- **In flight:** nothing. `serve-unknown-server-key-schema-drift` shipped and merged.
- **Last green slice:** `af98052` on `wave-serve-unknown-server-key-schema-drift`, merged into
  `main` (`--no-ff`) as `3b8b501`. Base was `main` at `5ac6c05`.
- **What shipped:** In `internal/codingagent/settings.go`, `checkServerRaw`'s known
  `settings.server` inner-key set is now derived at runtime from `ServerSettings`' fields via the
  new `isServerSettingKey` helper, replacing the hard-coded `port`/`hostname` case. The helper
  mirrors `encoding/json` field resolution: a tagged field by its JSON name, an untagged or
  `json:",opt"` field by its Go field name, and unexported / `json:"-"` fields not at all. A future
  `ServerSettings` field — tagged or not — can no longer land in `serverUnknownKeys` (the spurious
  "unrecognized server setting" warning) or trip the `allow…root` near-miss refusal. `allowedRoots`
  array-of-strings validation and case-folding behavior are unchanged. No test added (the fix
  enforces the coupling structurally). Spec: `docs/plans/2026-10-07-serve-unknown-server-key-schema-drift.md`.
- **Verification:** spec fan 5 angles, no blockers; Rev-2 folded the untagged-field gap (the helper
  now falls back to the Go field name). A temporary red-proof (added tagged + untagged fields to a
  scratch copy, ran a probe test, restored) confirmed neither leaks into `serverUnknownKeys` while
  `allowedHosts` still does. Diff fan 3 angles (scope+tracker, behavior, gate evidence) clean, 0
  blockers; the gate-evidence angle independently reproduced all gates at `af98052` with `-count=1`
  and a cold lint cache. Gates green in the worktree and again on merged `main`: `go build ./...`,
  `go vet ./...`, `go test ./internal/codingagent ./cmd/wopr` (255 passed), lint (0 issues),
  `bash -lc 'make check'` all green.
- **Accepted residuals:** the helper walks top-level fields only — if `ServerSettings` ever embeds a
  struct, its promoted fields would not be walked (documented `ponytail:` ceiling and upgrade path
  in the helper). `ServerSettings` is flat today.
- **Surfaced:** none new. The serve fail-open batch (`serve-allowed-roots-unknown-key-fail-open`,
  `serve-server-key-near-miss-fail-open`) and this schema-drift follow-up are all DONE; no READY
  follow-up remains from that batch.
- **tmux PATH nuance:** `tmux` lives on PATH only for login shells (`bash -lc`); run `make check`
  via `bash -lc` or it can fail at `test-prereqs`.
- **Remote:** `main` is ahead of `origin/main`; nothing has been pushed.

This file lives at the primary checkout on purpose. `wave` runs inside
`.worktrees/wave-<id>/`, which merge prunes; a resume point written inside a
worktree dies with it. Write it here, at `/home/kc/dev/wopr/RESUME.md`, from any
worktree.
