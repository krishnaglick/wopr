# Resume point

**KKCode phone test in progress — server side fixed, awaiting a real-app retry.** 2026-10-07

- **In flight:** nothing uncommitted. On top of the KKCode compatibility work
  (`e7569dc` routes, `78773a2` `/file` shape, `b6b76e2` DTO-shape parity, `d3df1be` replay test),
  two more fixes landed: `a0777d5` adds `/project` `time`/`sandboxes` and `/config.permission`
  keys, and `4642887` fixes `AuthenticatedProviders` — it only named a hard-coded provider list,
  so a provider authed solely via `auth.json` (`opencode-go`) had *no* models in the picker or
  `wopr --list-models`. `GET /config/providers` now lists all 36 opencode-go models.
  **Next:** owner retests KKCode (folder → session → prompt → stop) at `http://100.109.237.16:4098`;
  on any error read `/tmp/wopr-proxy.log`.
- **Owner config changed (backed up):** `~/.wopr/agent/settings.json` and `router.json` were edited
  to default to the working `opencode-go/deepseek-v4.1-flash` (the prior default `space-bunny-free`
  is dead upstream — `"Model is unavailable"`). Backups: `settings.json.bak-2026-10-07`,
  `router.json.bak-2026-10-07`. The proper interface is the interactive `wopr setup`; the edit is a
  stopgap. Verified: `/session/{id}/message` replies, `abort` → `session/status` `idle`.
- **Live rig:** `wopr serve` (rebuilt, `make install`) on `100.109.237.16:4097` → `/tmp/wopr-serve.log`;
  Node logging proxy `/tmp/wopr-proxy.js` on `:4098` → `:4097` → `/tmp/wopr-proxy.log`; real opencode on
  `:4096`. An iOS emulator is NOT available (Linux host, no Mac on the tailnet, closed-source App Store
  app is FairPlay-encrypted); the protocol-replay harness is the automated substitute.
- **Last green slice:** `4eb51e1` (SSE payload-envelope fix) on `main`; `make check` green.
- **SSE fix (`4eb51e1`):** opencode wraps every `/global/event` frame as
  `data: {"payload":{"id","type","properties"}}` with no `event:` line; wopr sent a bare flat
  payload behind `event:`, so a client reading `data.payload` threw on the first frame — the exact
  "cannot convert undefined value to object" KKCode hit on send. `sseWrite` now emits the envelope,
  `session.status` nests `{status:{type}}`, and `session.idle` is emitted on settle. Captured
  opencode frames (`/tmp/oc-sse.txt`) vs wopr frames confirm the match.
- **Full V2 parity is NOT needed for KKCode:** the capture proves it is a V1-contract client
  (`/global/health`, `/global/event`, `/session`, …; V2 only for `/api/health` and `/api/command`).
  The V2 plan stays parked as future work for genuinely-V2 clients.
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
