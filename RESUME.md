# Resume point

**Idle — ocean loop stopped: no dispatchable item left. One owner decision open.** 2026-10-07

- **In flight:** nothing. The session-directory plan is implemented; its three
  serve-allowlist follow-ups also landed. The only non-DONE item is NEEDS-INPUT:
  `serve-allowed-roots-unknown-key-fail-open` (on a misspelled `server.allowedRoots`
  key — refuse to start or warn). Ocean stopped rather than pick a favourite.
- **Last green slice:** `serve-allowed-roots-wiring-test` committed on
  `wave-serve-allowed-roots-wiring-test` as `06a850a`, merged into `main`
  (`--no-ff`). Base was `main` at `d57d826`.
- **What shipped:** Test-only. `TestGlobalServeAllowedRootsReadsGlobalLayerOnly`
  (`cmd/wopr/serve_test.go`) proves a `projectTrusted=true` project
  `.wopr/settings.json` with a broader `server.allowedRoots` cannot widen what
  `globalServeAllowedRoots` returns — a guard asserts the merged `sm.Get()` view
  genuinely contains the project root, so the assertion fails if the helper ever
  reads merged settings. `TestRunServeCommandRefusesRootOutsideGlobalAllowlist`
  pins `runServeCommand` to the global-derived list: a `--dir` outside it is
  refused with `is outside every server.allowedRoots entry`, catching a dropped
  `resolveServeRoot` call. No production file changed. Spec:
  `docs/plans/2026-10-07-serve-allowed-roots-wiring-test.md`.
- **Verification:** spec fan 3 angles (runtime-seam/falsifiability,
  convention-feasibility, completeness), all CLEAN with no blockers; accepted
  findings corrected the spec (Test 2's project fixture was inert under
  `projectTrusted=false`, so it was removed and the test renamed; its dropped-call
  red-proof was made a required gate). Implementer red-proofed both tests. The
  orchestrator independently reproduced both FAILs and gates: `go build ./...`,
  `go vet ./...`, `go test ./cmd/wopr ./internal/codingagent` (247 passed),
  `go tool golangci-lint run ./cmd/wopr/...` (0 issues), `bash -lc 'make check'`
  all green. Diff fan 2 angles (falsifiability; scope/hygiene), both CLEAN, 0
  findings. Note: the `rtk` proxy reported exit 7 for the lint run while printing
  "No issues found"; the direct `go tool golangci-lint` run exited 0.
- **Prior shipped:** `serve-allowed-roots-type-mismatch-fail-open` (`c7e9f0b`,
  merged `d57d826`); `serve-allowed-roots-fail-open` (`09bbd65`, `c0e8624`);
  `router-clock-seam` (`198e2da` + `861cfa5`, merged `57a2230`).
- **Tracker:** all items DONE except `serve-allowed-roots-unknown-key-fail-open`
  (NEEDS-INPUT); `serve-file-list-directory-param` FOLDED; `serve-7` DONE by
  decision.
- **tmux PATH nuance:** `tmux` lives on PATH only for login shells (`bash -lc`);
  run `make check` via `bash -lc` or it can fail at `test-prereqs`.
- **Remote:** `main` is ahead of `origin/main`; nothing has been pushed.

This file lives at the primary checkout on purpose. `wave` runs inside
`.worktrees/wave-<id>/`, which merge prunes; a resume point written inside a
worktree dies with it. Write it here, at `/home/kc/dev/wopr/RESUME.md`, from any
worktree.
