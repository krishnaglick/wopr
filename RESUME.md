# Resume point

**Idle — no wave in progress.** 2026-10-07

- **In flight:** nothing. `serve-kkcode-dto-shape-parity` shipped on
  `wave-kkcode-dto-shape-parity` and merged into `main` (`--no-ff`, `b6b76e2`) as this wave.
  Base was `main` at `3eb5c29`.
- **Last green slice:** `70c78f4` on `wave-kkcode-dto-shape-parity`.
- **What shipped:** Aligned wopr's V1 DTOs to opencode 1.18.34's captured shapes so KKCode stops
  aborting with "cannot convert undefined value to object". `GET /config` is the full 12-key object;
  `GET /config/providers` is the structural `{providers:[{id,name,source,env,key,options,models}],
  default:{}}` (key always `""`); the session DTO gains `agent`, `model{id,providerID,variant}`
  (never null), `path`, `project{id,worktree}`, `summary{additions,deletions,files}`, and
  `tokens{reasoning,total}`; `/agent` items are the 13-field captured shape; message info gains the
  full key set incl. `path{cwd,root}` and `parts[].snapshot`; `message.updated` builds through the
  same `messageInfo` builder as the poll path. Spec:
  `docs/plans/2026-10-07-serve-kkcode-dto-shape-parity.md` (+ `-shapes.md`).
- **Verification:** spec fan 5 angles found 3 blockers + majors; Rev-2 and Rev-3 fixed them, then a
  focused seam re-check was CLEAN. Diff fan 4 angles (shape correctness, scope/security/regression,
  test integrity, runtime seam) all CLEAN, 0 blockers; two warnings folded in before commit. Gates
  green in the worktree and re-run on merged `main`: `go build ./...`, `go vet ./...`, `go fix -diff`
  empty, `go test -count=1 ./internal/codingagent/serve/...`, `go tool golangci-lint run
  ./internal/codingagent/serve/...` (0 issues), `bash -lc 'make check'` all green.
- **Accepted residuals:** disk-only sessions report the server's current `modelSpec`, not the
  session's own last model — recorded as new item `serve-session-model-from-change-entry`. The
  `config/providers` structural change breaks the old `grapeot` @ `2d0e7bd` V1 client by design
  (KKCode is the target). `session.tokens.reasoning` is `0` (wopr's accounting has no reasoning
  field). The `test-faux/faux-1` placeholder is reported verbatim, not masked.
- **Surfaced:** `serve-session-model-from-change-entry` (new READY item). `serve-test-faux-fallback-in-production`
  remains READY and is the item that makes the model truthful.
- **tmux PATH nuance:** `tmux` lives on PATH only for login shells (`bash -lc`); run `make check`
  via `bash -lc` or it can fail at `test-prereqs`.
- **Remote:** `main` is ahead of `origin/main`; nothing has been pushed.

This file lives at the primary checkout on purpose. `wave` runs inside
`.worktrees/wave-<id>/`, which merge prunes; a resume point written inside a
worktree dies with it. Write it here, at `/home/kc/dev/wopr/RESUME.md`, from any
worktree.
