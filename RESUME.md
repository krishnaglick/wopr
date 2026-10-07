# Resume point

**Idle — no wave in progress. `session-directory` plan verified; awaiting an
implementation decision.** 2026-10-06

- **In flight:** `docs/plans/2026-10-06-session-directory.md` (untracked) —
  launch-time directory selection (`--dir` for the CLI and `wopr serve`), a
  global-layer `server.allowedRoots` guardrail, and the `/file` 400 fix, in 3
  slices. Three independent verifier fans ran; the final pass reported 0 blockers
  and the four residual warnings/nits were corrected inline. **Next:** dispatch a
  wave to implement it (it is the only READY item), unless the owner first wants to
  review Q2 (relative CLI paths vs `--dir`) and Q3 (`--dir` + `--resume`
  precedence), which the plan records as assumed defaults.
- **Tracker:** `session-directory` READY; `serve-file-list-directory-param` FOLDED
  into it; `serve-7` DONE by decision (all tools open by default).
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
- **Owner decisions taken 2026-10-06:** `serve-7` — all tools open by default, no
  approval gate (the `/permission*` 501 stubs stay). `serve-file-list-directory-param`
  — reject a foreign `?directory=` with 400; folded into `session-directory`.
  `session-directory` — both CLI and serve, operator allowlist, one root per
  process, launch-time root. See `BACKLOG.md`.
- **tmux PATH nuance:** `tmux` 3.4 lives at `/home/kc/bin/tmux` and is on PATH only
  for login shells (`bash -lc`). Non-login tool shells miss it, which is why some
  waves reported `make check` blocked at `test-prereqs` and later ones ran it green.
  Both are honest; the difference is shell init. Do not read the earlier "tmux
  absent" notes as a contradiction of the later green `make check`.
- **Tracker note:** `BACKLOG.md` and `RESUME.md` were brought under version control
  at `7452fce`; the directory-work edits since are uncommitted in the working tree.
  `.worktrees/` is now gitignored (`43717b5`) and `.agents/skills/ocean/` was
  deleted. The new plan `docs/plans/2026-10-06-session-directory.md` is untracked.
  `main` is ahead of `origin/main`; nothing has been pushed.

This file lives at the primary checkout on purpose. `wave` runs inside
`.worktrees/wave-<id>/`, which merge prunes; a resume point written inside a
worktree dies with it. Write it here, at `/home/kc/dev/wopr/RESUME.md`, from any
worktree.
