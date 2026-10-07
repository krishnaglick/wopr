# Resume point

**Idle — no wave in progress.** 2026-10-07

- **In flight:** nothing. `serve-kkcode-v1-compat` shipped on `wave-serve-kkcode-v1-compat` and
  merged into `main` (`--no-ff`) as this wave. Base was `main` at `70a9c08`.
- **Last green slice:** `b3a010a` on `wave-serve-kkcode-v1-compat`.
- **What shipped:** Added the KKCode V1 compatibility delta to `internal/codingagent/serve`:
  `GET /api/health` returns 200 `{"healthy":true}` from its own type (no `version` field);
  `GET /experimental/session` re-registers the existing `listSessions` handler (same body as
  `GET /session`); `GET /config` returns `{"model": s.modelSpec}` (honest subset, no credentials);
  `GET /api/command` returns `{location:{directory,project:{id:"global",directory}},data:[]}`; and
  `GET /permission` flipped from 501 to 200 `[]`. Auth untouched (wopr never emits 401). Out of
  scope: the V2 `/api/*` surface beyond health and command. `docs/site/docs/server.md` endpoint table
  and counts (34 routes / 29 implemented / 5 stubs) updated. Spec:
  `docs/plans/2026-10-07-serve-kkcode-v1-compat.md`.
- **Verification:** spec fan 5 angles CLEAN, then folded 9 accepted findings into the spec; diff fan
  4 angles (contract correctness, test integrity, scope+invariants, docs consistency) all CLEAN,
  0 blockers. Gates green in the worktree with `-count=1`: `go build ./...`, `go vet ./...`,
  `go test ./internal/codingagent/serve/` (81 passed), `go tool golangci-lint run ./internal/codingagent/serve/...`
  (0 issues), `bash -lc 'make check'` all green. Re-run on merged `main` after the merge.
- **Accepted residuals:** `GET /config` uses `omitempty`, so an empty `modelSpec` yields `{}` rather
  than `{"model":""}` — deliberate (honest subset). The `/experimental/session` test compares decoded
  bodies, which pins the response shape but not handler identity.
- **Surfaced:** `serve-test-faux-fallback-in-production` (new READY item) — `cmd/wopr/serve.go` falls
  back to `test-faux/faux-1` when startup model resolution yields none.
- **tmux PATH nuance:** `tmux` lives on PATH only for login shells (`bash -lc`); run `make check`
  via `bash -lc` or it can fail at `test-prereqs`.
- **Remote:** `main` is ahead of `origin/main`; nothing has been pushed.

This file lives at the primary checkout on purpose. `wave` runs inside
`.worktrees/wave-<id>/`, which merge prunes; a resume point written inside a
worktree dies with it. Write it here, at `/home/kc/dev/wopr/RESUME.md`, from any
worktree.
