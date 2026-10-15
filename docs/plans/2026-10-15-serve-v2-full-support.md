# serve-v2-full-support — full contract coverage of opencode V2 `/api/*`

- Date: 2026-10-15
- Status: **Rev-0.** Every claim below was re-derived on 2026-10-15 from the pinned spec
  (§1) and from this checkout at `42d8cab`. No claim here rests on a file in `/tmp`.
- Supersedes: `docs/plans/2026-10-07-serve-v2-api-parity.md` (Rev-1, marked superseded).
  Where the two disagree, this document wins; §0 lists every disagreement.
- Backlog item: `serve-v2-api-parity` in `BACKLOG.md` (filed by this plan; the 2026-10-07 plan
  reserved the id but never filed it).
- Authority: `AGENTS.md` — opencode is the specification for `wopr serve`. A divergence survives
  only as a recorded decision with a stated reason.
- Scope decision (owner, 2026-10-15): **full contract coverage**. Every `/api/*` path answers a
  spec-valid response. Where wopr has the engine, the answer is real; where it does not, the
  answer is a spec-blessed stub carrying a named reason. No route may answer Go's bare 404.
  Literal implementation of all 142 operations is **out of scope** — see §3.3.

## 0. What the true-up found

| # | Rev-1 claim | Verified 2026-10-15 | Evidence |
|---|---|---|---|
| 1 | Model of record `/tmp/openapi-v2.json`, 537.9 KB, sha `17951689…` | **File gone.** The spec moved: 578,272 bytes, sha `3fcd666b597b746b…`, **118 paths / 142 operations / 285 schemas** (was 117 / 141 / 256) | §1 |
| 2 | "`Command.Info` = `{name,description,template}`" | `{name, description}`; required is `[name]` only | spec `Command.Info` |
| 3 | "the opencode V2 spec" pinned at `f77342d` seams | 1.18.34's own source has **zero** `/api` route literals — V2 is a separate product line, so `coding/host` style citations had drifted a week of commits | §10 |
| 4 | "no spec `NotImplemented` tag exists" | Correct, but **`ServiceUnavailableErrorEncoded` exists** and 25 operations document 503 — so the stub answer is spec-blessed, not invented | §4.3 |
| 5 | "`Success{location,data}` envelope" as the general shape | Envelope is **per-operation, not universal**: 40 ops use `{location,data}`, 40 return an inline object, 12 a bare `$ref`, 49 have no 200 JSON at all | §4.1 |
| 6 | D3 auth open; D2 `/api/status` open; D6 open | D3 **decided** by owner 2026-10-07 (ignore `Authorization`, never emit 401 — `BACKLOG.md`, `serve-kkcode-v1-compat` notes). D2 **moot**: `/api/status` is in no spec and is a KKCode fallback probe only. D6 **answered by the spec** (§4.3) | §6 |
| 7 | Citations "re-read at `f77342d`" | All §7 and §10 citations re-derived today at `42d8cab` | §10 |
| 8 | — | **New:** wopr already serves two `/api/*` routes, and `GET /api/command` violates the V2 schema today (§5). This is the only route where a V1 client and the V2 spec collide | §5 |

## 1. The pin

| Field | Value |
|---|---|
| Source URL | `https://opencode.ai/v2/openapi.json` |
| Fetched | 2026-10-15 |
| sha256 | `3fcd666b597b746bad26f56a92fe782cc86338a0b4498f43b435a2f97e4fa05a` |
| Size | 578,272 bytes |
| Counts | **118 paths / 142 operations / 285 schemas** (201 of them `additionalProperties:false`) |
| `info` | title `opencode HttpApi`, version `0.0.1`, description *"Experimental HttpApi surface for selected instance routes."* |
| `servers` | empty array |
| `security` / `securitySchemes` | `[]` / absent — the spec documents 401 on all 142 operations and defines no scheme (§6 D3) |

**Do not vendor the JSON.** The repo does not vendor specs and a 578 KB blob rots the same way a
`/tmp` path does. The pin is this table plus the inventory in §2; re-derive it with:

```bash
curl -sSL https://opencode.ai/v2/openapi.json -o /tmp/openapi-v2.json
sha256sum /tmp/openapi-v2.json
```

When the hash moves, the spec changed: re-read §2 and §4 against the new file before implementing
against it, and treat any operation this document does not mention as **unclassified** (it must
land in T2 with a reason, or the definition of done in §3.2 is not met). The generator described in
`docs/plans/2026-10-09-serve-v1-conformance-and-parity.md` §S1 is the durable fix for this loop
and is not built yet; until it is, this re-derivation is a manual gate on S1.

## 2. Inventory (142 operations)

Per-prefix operation counts at the pin:

| Prefix | Ops | Prefix | Ops | Prefix | Ops | Prefix | Ops |
|---|---|---|---|---|---|---|---|
| `/api/session` | 39 | `/api/experimental` | 29 | `/api/integration` | 11 | `/api/pty` | 7 |
| `/api/vcs` | 6 | `/api/credential` | 5 | `/api/shell` | 5 | `/api/worktree` | 4 |
| `/api/permission` | 3 | `/api/plugin` | 3 | `/api/fs` | 3 | `/api/agent` | 2 |
| `/api/config` | 2 | `/api/debug` | 2 | `/api/location` | 2 | `/api/mcp` | 2 |
| `/api/model` | 2 | `/api/project` | 2 | `/api/provider` | 2 | `/api/websearch` | 2 |
| `/api/command` | 1 | `/api/event` | 1 | `/api/form` | 1 | `/api/info` | 1 |
| `/api/pair` | 1 | `/api/reference` | 1 | `/api/rpc` | 1 | `/api/skill` | 1 |
| `/auth/connect` | 1 | | | | | | |

Session surface (39), for the record: list, create, active, get, patch, delete, agent, background,
command, compact, context, diff, environment, fork, form (5: list/create/get/delete/reply),
generate, inbox (3: list/delete/patch), interrupt, message (2), model, move, permission (4),
prompt, revert (3), shell, synthetic, view.

**T1 — implement for real** (wopr has the engine; §7 names the seam):
`GET /api/info` · `GET /api/event` · `GET /api/session` · `POST /api/session` ·
`GET /api/session/{sessionID}` · `PATCH /api/session/{sessionID}` ·
`DELETE /api/session/{sessionID}` · `GET /api/session/active` ·
`GET /api/session/{sessionID}/message` · `GET /api/session/{sessionID}/message/{messageID}` ·
`POST /api/session/{sessionID}/prompt` · `POST /api/session/{sessionID}/interrupt` ·
`POST /api/session/{sessionID}/model` · `POST /api/session/{sessionID}/agent` ·
`GET /api/fs/list` · `GET /api/fs/read/*` · `GET /api/fs/find` · `GET /api/location` ·
`GET /api/project` · `GET /api/agent` · `GET /api/agent/{agentID}` · `GET /api/model` ·
`GET /api/model/default` · `GET /api/provider` · `GET /api/provider/{providerID}` ·
`GET /api/config` · `GET /api/reference` · `GET /api/skill` · `GET /api/command` (see §5) —
**30 operations.**

**T2 — stub in the V2 envelope, with a reason** (the remaining 112): PTY and persistent PTY
(7), worktree (4), VCS (6), MCP (2), credential (5), integration (11), plugin (3),
shell-as-a-service (5), websearch (2), `rpc` (1), `debug` (2), `pair` + `/auth/connect` (2),
session `form` (5), session `inbox` (3), session `permission` (4), session `revert` (3),
session `fork`, `diff`, `compact`, `context`, `environment`, `generate`, `move`, `shell`,
`synthetic`, `view`, `background`, `command`, `location/reload`, `config/shell`,
`project/{projectID}` patch, `permission/saved` (3), and all 29 `/api/experimental/*`.

Each reason is one of: *no engine in wopr* (pty, worktree, vcs, mcp, credential, integration,
plugin, shell), *product decision already deferred* (`BACKLOG.md` `serve-7`: forms and saved
permissions are all-open), *new persistence subsystem* (inbox, `session.next` log), *experimental
or off the chat path* (`/api/experimental/*`, `rpc`, `debug`, `websearch`, `reference` is in T1),
*deliberate* (pairing; see §6 D3). A T2 answer with no reason in `BACKLOG.md` does not count.

**T3 — namespace conflicts** (2 routes, §5): `GET /api/health` is served by wopr and is **absent
from the V2 spec**; `GET /api/command` is served by wopr with a body the V2 schema rejects.

## 3. Scope

### 3.1 Definition of done

1. Every path in §2 is classified T1/T2/T3 in `docs/site/docs/server.md`, and the doc names the
   spec sha256 it was classified against.
2. Every T1 operation answers a spec-valid body: exact field names, required fields present, and
   no extra fields where the schema sets `additionalProperties:false` (201 of 285 schemas do).
3. Every T2 path answers `503` + `{"_tag":"ServiceUnavailableError","message":"<reason>"}`. No
   route answers Go's default 404, 405, or an HTML error page.
4. V1 routes are byte-unchanged; the V1 suite passes unedited.
5. `internal/codingagent/serve` still imports nothing from `coding/`, and `coding/`, `agent/`,
   `ai/`, `tui/` are untouched. If a slice needs one of those, the seam is wrong (§7).

### 3.2 Why "full" stops here

Rev-1 rejected literal parity because parity here means implementing PTY, worktrees, VCS, MCP,
credential and integration management, plugin RPC, shell-as-a-service, pairing, a durable inbox,
and a form engine — subsystems wopr has no engine for, each of which breaks invariant 3.2.5 above.
Coverage is the part that decides whether a client works against wopr: a client gets parseable
answers everywhere, and real behavior everywhere wopr can honestly provide it.

### 3.3 Explicitly out of scope

Replacing or removing V1 (`/global/*`, `/session*`, `/file*`, `/find/*`, `/project*`, `/config/*`,
`/provider`, `/agent`, `/question*`, `/permission`) — V2 is added alongside. Auth, TLS, CORS,
rate limiting, audit. `POST /api/vcs/init`, the one operation the spec documents as 501. The
`session.next.*` durable event log. Replacing `/api/health`'s existing V1-era meaning (§5).

## 4. Wire facts (verified at the pin)

### 4.1 The success envelope is per-operation

40 operations return `{location, data}`; 40 return an inline object; 12 return a bare `$ref`
(`ServerInfo`, `PairingCode`, `PairingSession`, `Location.PublicInfo`, `SessionsResponse`,
`SessionGenerateResponse`, `SessionInterruptResponse`, `SessionMessagesResponse`,
`GenerateTextResponse`, `Project`, `Rpc.Output`, `Worktree.List`, `Worktree.Info`); 49 have no 200
JSON (204s and the SSE stream). **Read the envelope off the operation, not off a house style.**
A DTO shared across operations cannot assume `{location,data}`.

### 4.2 The error envelope

`{"_tag": "<ErrorName>", "message": "<text>"}`, always with `additionalProperties:false`; some
tags add `kind`, `field`, or `service` (all nullable). 35 `*Encoded` error schemas exist. Emit
`_tag` + `message` only — a tag's optional field is not required.

### 4.3 Status codes, as documented by the spec

| Code | Operations | Note |
|---|---|---|
| 400 | 142 (all) | the only universally documented error |
| 401 | 142 (all) | **with no security scheme anywhere** — see D3 |
| 404 | 121 | |
| 503 | 25 | `ServiceUnavailableErrorEncoded` exists → **the T2 stub is spec-blessed** |
| 200 / 204 | 96 / 46 | |
| 409, 500, 403, 413 | 13, 8, 4, 1 | |
| 501 | 1 (`POST /api/vcs/init`) | |

### 4.4 Events

`GET /api/event` is SSE. Each frame is `{id?, event, data}` with `additionalProperties:false`, and
**`data` is a JSON *string*** — `V2EventEncoded` is `{"type":"string","contentMediaType":"application/json"}`.
So the spec's own shape is a double-encoded payload: serialize the event object and emit the
resulting JSON as the frame's `data` string. Rev-1's D4 recommendation (a) is correct, and is now
verified rather than assumed. The stream's `x-effect-stream.causeSchema` documents an error channel
carrying the error envelope as the stream's terminal cause.

### 4.5 DTO burden

`Session.Info` requires `id, projectID, cost, tokens, time, location` (16 properties; wopr has no
reasoning-token count → emit the true value where known, `0` otherwise). `Model.Info` requires
`capabilities, variants, time, cost, status, enabled, limit` and has 18 properties — `variants`
and `status` have no wopr source, emit `[]` and `"active"`; this is the largest single DTO in the
slice and needs a round-trip decode test. `Agent.Info` requires `request` and `permissions` —
emit an empty `Provider.Request` and `[]`. `FileSystem.Entry` requires only `path` and `type`.
`ServerInfo` requires `version, pid, urls, paths`.

## 5. Namespace conflicts (T3)

V2 owns `/api/*`. wopr already serves two routes there, both added for KKCode during V1 work:

- **`GET /api/health`** (`serve.go:65`, handler `serve_handlers.go:95`) returns
  `{"healthy":true}` and is **not in the V2 spec at all**. It stays exactly as it is: it is a
  V1-era KKCode handshake, and V2 clients that want a health probe are served by the T2 catch-all
  (which answers a parseable error rather than a 404). Recorded here so nobody "fixes" it into a
  404 later.
- **`GET /api/command`** (`serve.go:96`, handler `serve_handlers.go:125-134`, DTO
  `serve_dto.go:322-330`) is the one live schema violation. V2 declares its 200 as
  `{location: Location.PublicRef, data: Command.Info[]}` where `Location.PublicRef` is
  `{directory}` with `additionalProperties:false`. wopr emits `location.project` — an extra key in
  a closed schema — and an empty `data` array. A strict V2 decoder rejects the response.

  **D7 (open):** options — (a) drop `location.project` and accept that KKCode reads a smaller
  object (its use of the route is a connect-time probe, `BACKLOG.md` `serve-kkcode-v1-compat`);
  (b) keep `project` and record a divergence; (c) serve `/api/command` from a separate V1
  handler and register the V2 shape last. **Recommend (a)**: it is the only option that satisfies
  both, and it deletes a key no client has been shown to need. Verify against a KKCode capture
  before merging.

## 6. Decisions

| # | Question | Status |
|---|---|---|
| D1 | Capture KKCode's real V2 traffic | **Open, lower priority.** The V1 capture (`serve-kkcode-v1-compat`) answered the V1 handshake completely and is why the V2 plan was deferred for that client. Revisit if a V2 client appears. |
| D2 | `/api/status` | **Closed — do not implement.** Absent from the spec at the pin; KKCode only probes it as a fallback when `/api/health` fails, and opencode answers it with web-UI HTML. |
| D3 | Auth | **Decided 2026-10-07 by owner:** ignore `Authorization`, never emit 401, keep every tool open. The spec's universal 401 with no security scheme is a recorded divergence (`BACKLOG.md`, `serve-kkcode-v1-compat` notes). The optional `server.authToken` idea is dropped from this plan. |
| D4 | Event `data` shape | **Closed:** emit the spec literally — `data` is a JSON string (§4.4). |
| D5 | Forms ↔ `askuser` | **Open, deferred.** The spec's form model is a typed-field engine wopr does not have. Recommend: T2 stub for `/api/form*` and session form (5 operations), and let questions flow through `question.v2.asked` on the event stream, which wopr already emits. Revisit if a client drives questions as forms. |
| D6 | Stub status | **Closed by the spec:** `503` + `ServiceUnavailableErrorEncoded` (§4.3). No 501 in the V2 surface except `POST /api/vcs/init`, which wopr does not serve anyway. |
| D7 | `/api/command` shape | **Open** — §5. |
| D8 | V2 identifier format | **Open, low priority.** The spec declares `^ses` and `^msg_` patterns; wopr mints UUIDs. wopr never validates its own ids, so a server is conformant. Revisit only if a strict client rejects an id. |

## 7. Capability mapping (citations re-derived 2026-10-15 at `42d8cab`)

| V2 operation(s) | wopr seam | Note |
|---|---|---|
| `GET /api/info` | `coding/version.Version`, `os.Getpid`, listener address | needs `version`, `pid`, `urls`, `paths`; `capabilities.persistentPty=false` |
| `GET /api/event` | `Host.Subscribe` `coding/host/host.go:139`, `forwardEvents` `:159`, `Bus.Publish` `coding/host/bus.go:52` | one producer; the V2 encoder re-encodes V1 bus events (§4.4) |
| `GET /api/session`, `GET /api/session/active` | `Host.Sessions()` `host.go:78`, `listSessionsFiltered` `serve_handlers.go:154` | new `Session.Info` DTO (§4.5) |
| `POST /api/session` | `Host.Start` `host.go:98` | |
| `GET`/`PATCH`/`DELETE /api/session/{sessionID}` | session read + write helpers, `serve_handlers.go:154-456` | delete maps onto the V1 archive-or-hard-delete decision |
| `GET .../message`, `.../message/{messageID}` | `sessionMessages` `serve_handlers.go:919`, `entriesToMessages` | cursor semantics are V2-only |
| `POST .../prompt` | `Session.PromptContent` `host.go:314`, `PromptContentWait` `:379` | per-prompt `system` staging already exists: `serve_prompt.go:104,161` |
| `POST .../interrupt` | `Session.Abort` `host.go:450` | |
| `POST .../model`, `.../agent` | `applyModel` / `SetModel` in `serve_prompt.go` | 204 in V2 |
| `GET /api/fs/list`, `/api/fs/read/*`, `/api/fs/find` | `resolveInRoot` `serve_dto.go:380`, `os.OpenInRoot` `serve_handlers.go:656,787`, walk `findFiles` `:838` | `FileSystem.Entry` is `{path,type}` — thin |
| `GET /api/location`, `GET /api/project` | `Services.CWD()`, `projectID`, `EvalSymlinks` | `Location.PublicRef` is `{directory}` |
| `GET /api/agent`, `/api/agent/{agentID}` | `listAgents` `serve_handlers.go:532` | V2 requires `request` + `permissions` |
| `GET /api/model`, `/api/model/default` | `AvailableModels` | heaviest DTO in the slice |
| `GET /api/provider`, `/api/provider/{providerID}` | `listProviders` `serve_handlers.go:458` | |
| `GET /api/config` | `getConfig` `serve_handlers.go:100` | |
| `GET /api/reference`, `/api/skill` | `internal/codingagent` skills + context files | |
| `GET /api/command` | `apiCommand` `serve_handlers.go:125` | see D7 |

Shared wiring: V1 mux `serve.go:63-116`; `serve.New` `serve.go:41`; the call site
`cmd/wopr/serve.go:192`; `writeJSON` `serve_handlers.go:39`; `notImplemented` `:80`;
`ServerSettings` `internal/codingagent/settings.go:180`.

## 8. Slices

Each is one feature, gates green, independently committable. Run in `.worktrees/wave-serve-v2-*`.

**S1 — inventory classifier.** Classify all 142 operations T1/T2/T3 into a committed
`internal/codingagent/serve/testdata/opencode-v2-inventory.json` (`path`, `method`, `tier`,
`reason`), generated by a script that fails loudly when the resolved count is below the raw
`operationId` count in the fetched spec. `docs/site/docs/server.md` gains the sha256 and a pointer
to it. **This is the V1 conformance S1 generator, pointed at V2** — build it once, use it twice.
Gate: a test asserting 142 classified and no duplicates; re-run the generator and confirm an
unchanged file.
**S2 — handshake and envelope.** `/api/*` catch-all, the error envelope, `/api/info`, and the
existing `/api/health`/`/api/command` handling under it. *Connection test.*
**S3 — session read and lifecycle.** list/create/get/patch/delete/active + the `Session.Info` DTO.
*Session picker.*
**S4 — prompt loop.** `/api/event` encoder + prompt/interrupt + message read. Adds the **manual
live-curl gate**: this package's suite is historically blind to wire defects (five escapes
recorded in `BACKLOG.md` serve-3/serve-4), so curl against a real listener is load-bearing.
*Chat loop.*
**S5 — read surface.** fs, location, project, agent, model, provider, config, reference, skill,
command. *Pickers.* D7 lands here.
**S6 — forms, permission, and docs.** The T2 stubs, the threat-model and settings updates, and
`server.md`'s endpoint table.

Per slice: `go build ./...`, `go vet ./...`, `go test -count=1` on touched packages,
`go tool golangci-lint run` on touched packages, `go fix -diff ./...` empty. Before merge:
`make check`.

## 9. Risks

- **The spec is experimental and moving.** `info.version` is `0.0.1`, the description says
  "Experimental", and it gained a path, an operation, and 29 schemas in the eight days between
  the two pins. S1's classifier is the only defence; treat an unexpected operation count as a
  blocker, not a merge conflict.
- **Spec, SDK, and server disagree** (recorded in Rev-1 and still true): the spec has `/api/info`
  and no `/api/health`; a strict decoder will reject wopr's `/api/command` today. Build against the
  spec, tolerate added fields on input, emit no extras on output.
- **`additionalProperties:false` on 201 of 285 schemas** turns every placeholder decision into a
  client-visible failure. Round-trip decode a representative payload for each new DTO.
- **The suite is not evidence here.** Green tests have coexisted with live bugs in this package
  five times. The S4 curl gate and the S1 classifier are the two things that catch what the tests
  miss.
- **Namespace crowding.** `/api/*` now holds V1-era routes and V2 routes. New V2 work must not
  assume the namespace is clean.

## 10. Verification

- The pin: sha256 and counts in §1, re-derived by the curl in §1. Record the same hash in
  `docs/site/docs/server.md` when S1 classifies the inventory, so the number a reader checks is on
  the page that claims conformance and not only in a plan.
- Every `file:line` in §7 and the wiring paragraph was re-read on 2026-10-15 at `42d8cab`.
- The V2 spec is **not** readable in this repo's local opencode checkout: `opencode-src` is pinned
  at v1.18.34 (`aec0b9a`), whose route groups contain no `/api` literals. That checkout is the
  authority for V1; this document's authority is the fetched spec.
- Next: `verify-fan` over this document before S1 dispatch, with the spec file pinned for the run.