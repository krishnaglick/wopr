# Resume point

**Idle — no wave in progress. `router-clock-seam` shipped and merged.** 2026-10-06

- **Last green slice:** `router-clock-seam` committed on `wave-router-clock-seam`
  as `198e2da` (+ `861cfa5` spec-citation fix), merged into `main` as `57a2230`
  (`--no-ff`). Branch and worktree pruned after a verified merge. Base was `main`
  at `e1f4ac2`.
- **What shipped:** Added `Router.SetClockForTest(func() time.Time)` — a
  mutex-guarded, test-only clock seam; `New` still defaults to `time.Now` and no
  production cooldown behavior changed. `TestSessionFailoverRestsTheFailedModelForQuotaBackoff`
  (`coding/session_router_failover_test.go`) drives a `free_tier_limit` failover
  through the real session (`faux-1`→`faux-2`), asserts the post-failover decision
  is `faux-2` and `At` is the fake clock (anti-vacuous-pass guards), advances 61s,
  and asserts `faux-1` is not re-offered — i.e. it rested `quotaBackoffMinutes`
  (15m), not the generic 60s. **Divergence flagged:** the test lives in
  `coding/session_router_failover_test.go`, not the item's named
  `coding/session_retry_classify_test.go`; the backlog `Files:` line was
  reconciled on ship. Spec: `docs/plans/2026-10-06-router-clock-seam.md`.
- **Verification:** spec fan 2 angles (runtime-seam, leanness/test-oracle), both
  CLEAN, 0 blockers; accepted findings (seam renamed `SetClockForTest`, comment
  states the unlocked-read concurrency fact, spec-prose accuracy) corrected before
  dispatch. Diff fan 2 angles (gate-integrity/scope-fit, test-oracle/seam),
  both CLEAN; both flagged the spec's stale mutation line citation, fixed in
  `861cfa5`. RED: compile-red (`SetClockForTest undefined`) before the seam;
  mutation-red (`Next`'s RateLimited cooldown → `time.Minute`) fails with
  `faux-1 re-offered after 61s`, reproduced and reverted by the orchestrator and
  independently by both verifiers. Gates on branch and re-run on merged `main`:
  `go build ./...`, `go vet ./...`, `go fix -diff` (clean), `golangci-lint` (0
  issues), touched-package tests + `-race`, new test `-count=3`, `go test ./...`,
  docs-drift, and **`make check` fully green — tmux IS installed (3.5+)**, contrary
  to earlier resume notes.
- **Prior shipped:** `serve-ci-host-race-echo` committed `369fa93`, merged
  `e1f4ac2`; `docs-drift-link-syntax-ceiling` committed `ac00a15`, merged
  `1d41da1`; `docs-drift-docs-json` committed `2625071`, merged `494fa4d`;
  `serve-cli-doc` committed `8186386`, merged `d2842ec`.
- **Backlog dry of dispatchable items.** `serve-file-list-directory-param` was
  moved READY → NEEDS-INPUT (does a foreign `?directory=` mean anything when the
  server exposes one root? reject 400 vs accept-and-ignore). `serve-7` remains
  BLOCKED on a human product decision. These are the owner's queue — the ocean
  loop stopped here rather than picking a favourite. See `BACKLOG.md`.
- **tmux PATH nuance:** `tmux` 3.4 lives at `/home/kc/bin/tmux` and is on PATH only
  for login shells (`bash -lc`). Non-login tool shells miss it, which is why some
  waves reported `make check` blocked at `test-prereqs` and later ones ran it green.
  Both are honest; the difference is shell init. Do not read the earlier "tmux
  absent" notes as a contradiction of the later green `make check`.
- **Tracker note:** `BACKLOG.md` and `RESUME.md` are untracked in this repo;
  this wave's tracker edits live in the working tree, not in a commit.
  `.worktrees/` (now only `wave-serve-ios`), `.agents/skills/ocean/`, and
  `docs/plans/2026-10-03-serve-opencode-ios.md` are likewise untracked /
  pre-existing dirty on `main`.

This file lives at the primary checkout on purpose. `wave` runs inside
`.worktrees/wave-<id>/`, which merge prunes; a resume point written inside a
worktree dies with it. Write it here, at `/home/kc/dev/wopr/RESUME.md`, from any
worktree.
