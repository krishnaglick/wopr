# wopr backlog

Items are picked up cold by a session that has never seen this repository. Status is
load-bearing: `READY` is the only status that gets dispatched, and nothing is `DONE`
until its gates were verified rather than merely dispatched.

The plan of record for the current batch is
`docs/plans/2026-10-03-serve-opencode-ios.md`. Each item below is one of its slices,
and the gates are the repo's standard set from `AGENTS.md`: `go build ./...`,
`go vet ./...`, `go tool golangci-lint run` on the touched packages, `go test -count=1`
on the touched packages, `go fix -diff ./...` empty, and `make check` before merge.

## Standing constraints for every item in this batch

- Wrap toolchain commands in `bash -lc` — non-login shells have a sanitized PATH and
  `go`, `make`, and `tmux` all appear missing.
- Never modify `loadSessionFile` retention in
  `internal/codingagent/session_manager.go`. `raw` is write-critical:
  `SessionEntry.MarshalJSON` returns it verbatim and the full-file flush reads every
  `raw` under a held mutex.
- `internal/codingagent/serve` must not import `coding`. `coding/services.go:18`
  already imports `internal/codingagent`; that would be an import cycle. The
  multi-session supervisor belongs in `coding/host`.
- `runGen` is `InteractiveMode`'s TUI-only field, and there are two different
  `queueMu`s in two packages. Neither is a server-side concurrency primitive.
- `tmux` is 3.4, so four integration tests run a documented degraded fallback. That is
  pre-existing; do not "fix" it by raising a skip threshold.
- No auth and no TLS in this batch. The path allowlist on resolved paths is what makes
  auth-less LAN serving defensible, so it ships before or with the file endpoints, not
  after.

---

### serve-1 — Supervisor, event bus, and listener

- **Status:** DONE — shipped on `wave-serve-1`, merged into `main`
- **Why:** Nothing can be served until a process owns more than one session and can
  publish events to more than one subscriber. This is the seam the whole plan rests on.
- **Files:** `coding/host/` (new: multi-session supervisor, typed event bus with an
  injectable sink), `internal/codingagent/serve/` (new: listener), `cmd/wopr/serve.go`
  (new: flags and wiring), `internal/codingagent/settings.go` (a `server` section for
  `port` and `hostname` only).
- **Gates:** one test that a second concurrent session prompt is rejected per session,
  and one that `/global/health` answers over a real listener. — both met.
- **Notes:** `coding.StartSession(svcs, opts)` is already a free function over a shared
  `*Services` and `Services` holds no session, so multi-session needs a supervisor, not a
  refactor. `services.go:18` already imports `internal/codingagent`, which is what forces
  the layering. Only `server.port` and `server.hostname` earn a settings key — the plan
  cuts `projectDir`, `maxSessions`, and `auth`. `http.Server{IdleTimeout}` handles
  keep-alive reaping; only the SSE heartbeat is new work.

**What landed, for the next session**

- `coding/host`: `Host` with an id-keyed session registry, `Start`/`Get`, and the run rule
  — a per-session `sync.Mutex.TryLock` taken before `Send`, refusing with `ErrSessionBusy`.
  A blocking `Lock` instead would deadlock the caller, because `sendContent` holds
  `s.mu` for the whole run (`coding/session.go:653,667-684`).
- `coding/host/bus.go`: `Bus.Publish` marshals once and hands the same `[]byte` to every
  subscriber. **Symbols are exported with no in-slice caller on purpose** — the repo's
  `unused` linter rejects unexported dead code, so unexporting fails the lint gate. Do not
  "fix" this. `Subscription`'s field is `doneCh` and its accessor `Done()`; same names
  collide and fail the build.
- `internal/codingagent/serve`: `New(addr)` binds and returns the `*http.Server` and
  `net.Listener`; `GET /global/health` answers `{healthy, version}` with
  `IdleTimeout: 120s` and `ReadHeaderTimeout: 5s`. It imports `coding/version`, **not**
  `coding`. The `serve -> coding/host` edge is still unbuilt; it lands with the first
  route that needs a session, which is serve-3's `POST /session`.
- `cmd/wopr serve --port --hostname`, flags over settings, `server.port` default 4096 and
  `server.hostname` default 127.0.0.1, usage errors exit 2 with a usage line, bind failure
  exits 1, and a non-loopback bind prints the exposure warning.
- Spec of record for this slice: `docs/plans/2026-10-04-serve-1.md`.

**Left open deliberately, and where it goes**

- The SSE heartbeat. The plan and this item's Notes name it slice-1 work; it was deferred
  to serve-4, on the plan's own "do not start SSE before a prompt streams correctly".
  Recorded as open question 1 in the slice spec, with the counter-arguments, so a human
  can overrule it.
- The bus has **no test beyond `Close` idempotency** and no subscriber. Its blocking-send
  and shared-payload invariants are comments only. serve-4 replaces the blocking send with
  the byte bound and the terminal drop event, and should prefer `Session.Subscribe`
  (`coding/session.go:1648`) plus one `json.Marshal` over a second subscription mechanism.
- `Publish` cannot observe a `json.Marshal` failure, and `Host.Get` still has no caller.
  Both are serve-4's to resolve when there is a caller.

### serve-2 — Point `--mode rpc` at the same supervisor

- **Status:** DONE — shipped on `wave-serve-2`, merged into `main` as `f7bd732`
- **Depends on:** serve-1
- **Why:** The dispatch switch is inlined in `runRPCMode` and everything reusable is
  unexported in `package main`, so RPC and a server would otherwise drift into two
  divergent engines.
- **Files:** `cmd/wopr/rpc_mode.go` (delete the inlined switch and the `os.Stdout`
  closure), `cmd/wopr/rpc_types.go`, `cmd/wopr/rpc_events.go`, plus the new `coding/host`.
- **Gates:** `go test ./cmd/wopr -count=1` green with **no edits to existing tests**; a
  test you had to edit here means the wire protocol moved, which is the exact failure
  this item exists to prevent. — met.
- **Notes:** 87 `writeRPC` call sites and roughly 20 captured locals come with it. The
  `os.Stdout` closure is also the seam the serialize-once fan-out rule needs. The four
  globals that are process-scoped today — one session, one set of wait groups, one
  `settlePrompt`, one `streaming()` — become per-session state owned by the host.

**What landed, for the next session**

- `coding/host/rpc.go`: `ServeRPC(ctx, svcs, sess, RPCOptions) int`. The inlined 33-arm
  dispatch switch is gone and the 87 `writeRPC(` sites plus 17 `decodeRPC` sites moved
  **unrewritten**, because `writeRPC := opts.Write` is bound as a local on the first line
  of the body. That single line is why the move is 20 identifier rewrites instead of ~124.
- Per-session state is now per-`ServeRPC`-invocation state: the four globals are locals and
  closures of one call, not struct fields of an engine object. **Not** wired into
  `host.Host` — the registry and the run rule still belong to `host.Start`, and nothing in
  this slice calls it.
- `coding/host/rpc_events.go`: `rpc_events.go` moved whole with one export rename,
  `rpcAgentEvent` -> `WireAgentEvent`. `print_mode.go` imports the package as `codinghost`
  because `runPrintMode`'s second parameter is named `host`.
- `cmd/wopr/rpc_types.go` is 2 type aliases plus `writeJSONLine`. `rpcCommandCatalog` and
  its **six** methods stay in `package main` because `llama_test.go:23` constructs the
  unexported catalog by name; `routePrompt` is reachable only from `llama_test.go:35`, so it
  must survive any "test-only dead code" pruning.
- Spec of record: `docs/plans/2026-10-04-serve-2.md`. A 5-angle fan drove three revisions;
  its verdict history is the argument for reading the spec before touching this code.

**Two things the gates could not see, and serve-3 and serve-4 inherit**

- **`go test ./cmd/wopr` has no teeth for a wire change.** Zero test files assert on
  response text, so the slice's own acceptance criterion would have passed over a byte-level
  change. The one that existed: `encoding/json` embeds `<pkg>.<Type>` in a type error, so
  moving the wire types out of `package main` changed every malformed-command error to
  `host.RPCCommandEnvelope`. `unmarshalRPC` in `coding/host/rpc.go` restores the
  qualifier on the two paths whose errors reach the wire — bytes, not accuracy, marked with
  a `ponytail:` comment. **Any future move of a wire type repeats this trap.**
- Byte-identity was confirmed empirically, not assumed: old and new binaries driven through
  60+ hermetic invocations, three arms each (old, new, old-again) to separate regression from
  per-run noise. Response shapes, the whole streaming path, stderr, and exit codes are
  identical. Concurrent `bash` replies and `prompt`-then-`abort` interleaving are
  nondeterministic at base and unchanged here, so **no byte-exact golden test over those two
  paths is possible** without a quiesce.

**Left open deliberately**

- The `os.Stdout` closure at `cmd/wopr/rpc_mode.go:156-161` is **retained byte-identical**,
  which is not the item's literal wording. `writeJSONLine` still has callers in
  `print_mode.go`, so deleting it outright was impossible; the requirement that actually
  matters — the engine names no writer — is met. Ruling and reasoning in the spec.
- `Host.Get` and `Bus.Publish` still have no production caller, and `ServeRPC` takes
  `*coding.Services` rather than a `*Host`. The `serve -> coding/host` edge is still
  unbuilt; it lands with serve-3's `POST /session`, and that is the first slice whose
  DoD needs a byte-level wire check that the test suite cannot provide.
- serve-4's `serve`-side call shape (`h.svcs`, `s.sess`) needs exported accessors that do
  not exist yet. Recorded as an open question in the spec.
- Teardown **ordering** is preserved by the verbatim copy but is not asserted anywhere;
  idempotency of the double `Session.Close` is explicitly **not** claimed
  (`coding/session.go:933` guards only `close(s.closeDone)`).
- Spec drift worth knowing: the spec says `rpc_events.go` moves "all 392 lines"; `rpcUsage`
  and `tryDecodeJSON` went to `rpc.go` instead, because leaving them behind would make them
  unexported dead code in `package main` and fail the `unused` gate.


### serve-3 — Session and read surface

- **Status:** **SHIPPED** — merged `--no-ff` into `main` as
  `Merge wave-serve-3: session and read surface`. Seven commits on `f7bd732`; the merge also carries
  the follow-up that fixed the defect below. Worktree pruned after a successful merge.
- **Depends on:** serve-1
- **Why:** The phone cannot list, open, or browse anything until the read endpoints
  exist, and every required DTO field has to be emitted or decoding fails on the client.
- **Files:** `internal/codingagent/serve/` handlers and DTOs.
- **Gates:** per-DTO tests that decode a representative payload; the bounded list read
  and the tail read land here with `BenchmarkServeFanout -benchmem`.
- **Notes:** The client requires `Session.slug` and `Session.version` as non-optional
  and has no wopr equivalent — derive both deterministically from the session ID. It
  sends `directory`, `limit`, `path`, and `query` query parameters that must all be
  answered. `PATCH` has two distinct bodies, rename and archive. `session.tokens` and
  `session.cost` are optional and cumulative: emit them or omit them, never zeros that
  read as real totals. `/session/{id}/todo` needs the `todowrite` tool from serve-5 to
  have anything to return. `DELETE` and `/fork` answer 501, which the client explicitly
  treats as not-implemented. Do not use `summarizeSessionFile` for the list endpoint: it
  retains `AllMessagesText` for every session listed, which is a larger greedy read than
  the loader this plan already avoids.

### serve-3 — What blocked the merge, and how it closed

One defect, reproduced against a live listener, not inferred:

**`PATCH` rename on a session the server did not create returns 200 and changes nothing.**
A `.jsonl` written directly into the resolved session dir — the state every session is in
after a server restart — is listed by `GET /session` and answered by `GET /session/{id}`,
but renaming it returns `200` with `"title":""`, and the following `GET` still reports
`"title":""`. The rename is a silent no-op.

This is the exact case the slice's own A2 ruling was built on: `Host.Resume` was withdrawn
precisely so that renaming a session the phone did not create would cost an append rather
than a transcript read. The append path works for a session the host owns
(`TestPatchThenGetAgrees` covers cold and live); it does not reach a session that exists
only on disk.

**The suspicion in this record was wrong, and the correct answer was three defects, not one.**
`SessionManager.RenameSession` is not a no-op when unbound — it appended `NEW-TITLE` correctly every
time. The file changed; the **reader** could not see it. Reproducing over a live listener found:

1. **`sessionRecordAt` walked oldest-first** (`serve_read.go:207`) while the plan specified
   last-writer-wins in three places. A rename read back the original name. Branch B already scanned
   `slices.Backward` (`:338`), which is exactly why a live session renamed correctly and a disk one
   did not, from the same handler.
2. **The `Archived` type switch had no `nil` arm** (`:224-234`, `:352-362`), so the `null` that
   clears an archive matched nothing, the open `archived == nil` guard fell through to an *older*
   record, and a cleared session still reported archived. **Both** readers, not just the disk one.
3. **The two file-level appends wrote `"%s\n"` with no separator check**
   (`session_manager.go:339`, `:366`). On a file not ending in a newline the new record was glued
   onto the last one — 200, and permanent. On the rename path this read as `"title":""` before and
   after, which is how the *original* symptom in this record was really produced: the glued line
   fails to parse, so the reader found no `session_info` at all. `/import` reaches it with no crash,
   since `sessionblob.Relocate` rejoins on the separator it split on.

Fixed by `slices.Backward`, a dedicated `archivedSet` flag separating "consumed" from "latched a
value", and one `appendJSONLine` helper (both opens become `O_APPEND|O_RDWR`; the last-byte read
needs a readable descriptor). `TestPatchRenameAndArchiveOnFileBackedSession` drives all of it over a
real listener, and every requirement was **seen red**: B1 alone turns requirements 2, 3 and 4 red,
B1b alone turns 3, B2 alone turns 5 on both appends. Five cells were also re-measured over a live
listener on all three revert states.

### serve-3 — Carried debt, recorded rather than fixed

Three findings from the defect work that nothing else records. All are real; none is this slice's.

1. **`Session.AppendCustomEntry` has the same missing separator**, with **seven** callers:
   `coding/session_council.go:75`, `coding/session_router.go:214`, `coding/session_queue.go:109`,
   `coding/session_goal.go:346`, `coding/session_efficiency.go:428`,
   `internal/codingagent/sidebar_snapshot.go:112`, `serve/serve_handlers.go:298`. One-line fix
   inside `appendSessionLine` (`session.go:567-577`); wider blast radius than the two callsites
   serve-3 fixed, and no measurement harness exists for it yet.
2. **`sessionRecordAt` and `sessionRecordFromLive` are two implementations of one rule**, deriving
   `Name` and `Archived` from `[][]byte` and `[]SessionEntry` respectively. They now agree — B1 and
   B1b made that true, and it was false before — but unifying them means normalising both to raw
   bytes. Note that `serve_read.go`'s own §4.2 heading, "Why the two branches cannot disagree", was
   **false** until this commit, for a different reason in each branch.
3. **A header-only file with no trailing newline 404s** at `serve_read.go:182`.
   `reader.ReadBytes('\n')` returns the unterminated line with `io.EOF` and `:183-185` rejects any
   error. Three-line fix — accept `io.EOF` when the line is non-empty, the same shape as the
   `len(headerLine) == 0` guard at `:187`. Left alone because wopr does not produce the state for a
   session it creates (`writeDerivedSession` writes the header with its newline; `Create` writes
   nothing), `/import` reaches it and gets a consistent 404 rather than corruption, and fixing it
   here would leave that probe cell with no measured value — the harness is scratch.

### serve-3 — What landed, for the next session

- 19 routes over 15 handler functions plus the existing health handler, the wire DTOs, and
  the `serve -> coding/host` edge. Four exported symbols in `coding/host` (`Host.Services`,
  `Host.Sessions`, `Session.Inner`, package-level `AvailableModels` and `ModelProviderID`).
- The session directory is resolved **once**, in `cmd/wopr/serve.go`, and used by both the
  reader and `Host.StartOptions.SessionDir`. `resolveSessionDir` returns `""` for the
  default config, so the empty case falls back to `codingagent.NewSessionManager(cwd).
  SessionDir()`. Do not reintroduce a second resolution order.
- The id-to-path lookup is `os.ReadDir` plus a suffix match on a validated id. It was a
  `filepath.Glob` first: the session dir was interpolated into the pattern unescaped, and a
  `*` anywhere in a project path matched a second project's session directory. **The dir is
  an argument, never a pattern.**
- Both reads are bounded and measured: the list read holds ~52.8k allocations at 400 and at
  800 entries per session; `tailEntries` on the pinned 17 MiB entry went from 255-290 ms to
  under a millisecond.
- Live-probe confirmed: `GET /session` lists a session the server did not create, both 501
  stubs answer 501, a symlink to `/etc/passwd` is refused 403, an absolute path is refused
  403, `/file/status` returns a tracked filename containing a space whole, empty lists are
  `[]` and not `null`, and a `PATCH` response agrees with the `GET` after it for a session
  the host owns.
- The merge-blocking round landed `slices.Backward` on the disk reader, an `archivedSet` flag in
  both readers, and one `appendJSONLine` separator helper. Re-measured over a live listener:
  6/6 cells green on the merged binary, and 1/6, 5/6 and 4/6 on the three single-revert
  binaries — so each of the three defects is caught by a cell the other two do not share.

### serve-3 — The pattern the next session must not repeat

Five verification rounds, each finding real defects that every gate had passed:

1. `walkBack` aliased every read block into one buffer, so the tail read returned 0 entries
   for a 17 MiB entry and silently dropped the newest 113 messages at `limit=200`.
2. `listSessions` drained its worker channel with a non-blocking `select`, so `GET /session`
   returned `null` — 15 runs out of 15.
3. `projectID` took twelve raw digest **bytes** instead of hex, putting invalid UTF-8 into a
   field the client requires. `tokens` and `cost` shipped as zeros, which this item's own
   note forbids by name.
4. `stopTail` counted read **blocks** against `limit`, so `limit=20` returned 4 messages when
   entries exceeded a block — silent under-delivery the client cannot tell from end-of-history.
5. `workingTreePaths` split porcelain records on whitespace, truncating any tracked filename
   containing a space.

Seven tests reported green while their bug was live, and round 2 showed that reverting both
the per-block clone and the drain left the suite green. **A green suite in this package is
not evidence.** Every defect that mattered was found by running the binary, or by
deliberately breaking the code underneath each test.

Round 6, the one that blocked the merge, kept that true — and then added a sixth way to be fooled:

6. **A gate was red for four rounds and nobody read it.**
   `internal/testenv/TestWindowsTestsCreateSymlinksThroughSymlink` had been failing on
   `serve_read_test.go:338` since commit `c5a7b34` — a direct `os.Symlink` in a test that
   builds for Windows. **This slice introduced it**, and five earlier verification rounds
   never ran `make check` to completion; they ran its parts. One line through the repo's own
   `testenv.Symlink` helper. The lesson generalises past this defect: `make check` is a gate,
   and running a subset of it is not running the gate.

### serve-3 — Two scope reductions, recorded rather than shipped silently

- `GET /session/{id}/diff` answers **501**. The plan does not mark it not-implemented
  (plan:132) while it explicitly marks `DELETE`, `/fork` and `/revert`, and slice 3 names
  `/diff`. wopr has no snapshot substrate and building one is far outside this slice. The
  client treats 501 as not-implemented (plan:147-151), which is what makes this defensible.
- **`/permission*` is assigned to serve-5, as 501 stubs** (decided 2026-10-04 by a
  four-dispatch fan; alternatives and evidence below). `GET /permission` (plan:142) and
  `POST /session/{id}/permissions/{id}` (plan:143) fall between slices: the plan's slice
  list (plan:299-334) names neither, and neither does serve-5's Files line. serve-5 owns
  them **as `notImplemented` handlers answering 501**, and the real substrate becomes
  `serve-7`. Today both routes are **unregistered and answer 404**, with no catch-all on
  the mux (`internal/codingagent/serve/serve.go:41-76`); 404 is not an honest answer,
  because the client reads it as "host gone".

  Why 501 and not a real implementation: the client treats 501 as not-implemented
  (plan:147-151), which is the same treatment serve-3 gave `/diff` above. The substrate is
  *more* invasive than revert's snapshot store, not less — three things must be invented
  that wopr has no home for: a policy surface saying what requires approval (Settings
  carries only `ShellPath` and `CommandPrefix`, `internal/codingagent/settings.go:77-80`);
  **the first hook in the tree that blocks on a human**, which lands in the agent loop's
  *serial* prep phase (`agent/tool_execution.go:103-121`) and so stalls every tool in the
  batch, not just the one awaiting a thumb (all five existing hook users return
  synchronously); and a pending-request store holding an id the model never saw. Decisive
  asymmetry against `/revert`: the client has **no `Last-Event-ID` and no replay**
  (plan:162-163), so a permission asked while the phone was out of range is unrecoverable
  — that is a liveness protocol needing its own persistence story.

  Why serve-5 rather than serve-4: serve-4 owns the *mapping* of all 17 SSE events
  (plan:318-319) while serve-5 is explicitly handed "the three question events"
  (plan:321-322) — **production sits with the feature slice**, not the transport. In-tree
  precedent: serve-3 shipped `GET /session/{id}/todo` as an empty-array stub whose producer
  is serve-5 (`serve.go:55`, `serve_handlers.go:398-407`). serve-5's asker half needs zero
  new engine code (`askuser.Tool` and `Session.SetUserAsker` already exist), so the stubs
  keep its three gates honest; folding an undesigned policy engine in would make its "done"
  mean two different things.

  Alternatives rejected, with costs: **a new slice carrying real 200-returning permissions**
  — re-derives a second near-identical pending store (the shapes differ only in path
  prefix, plan:143 vs :144) and needs a plan section, gates, and a CHANGELOG line; **real
  permissions inside serve-5** — imports all three undesigned decisions above plus a
  batch-concurrency test its gate list does not cover; **serve-4** — strictly worse, it
  would carry an unspecified approval protocol plus two permission SSE events *and* the
  prompt path, raising risk on the slice that "proves the seam" to buy nothing; **leave it
  at 404** — cheapest of all and rejected, because an unregistered path is an accident, not
  an answer; **serve-6** — it is docs-only (`plan:325-328`), and an endpoint in
  `internal/codingagent/serve` is not a doc.

  Unverified, and it is the one thing that would flip this: whether the client's capability
  check distinguishes 501 from absent **for these two routes specifically**. No Swift
  source exists in this repo. `plan:368-371` already names the ten-minute check — grep
  `grapeot/opencode_ios_client` at commit `2d0e7bd`. If the client treats 501 on
  `GET /permission` as fatal, the answer becomes "always-empty 200 array" instead.

  Two mechanical costs, both landing in **serve-5's** commit, not serve-3's: the slice
  spec's DoD asserts `grep -c 'mux.HandleFunc'` prints `20` and two routes make it `22`;
  and the `notImplemented` stub's comment says "the four 501 routes"
  (`serve_handlers.go:73`), which becomes stale. Serve-3 did **not** add these routes —
  it merged with the count verified at `20`, and serve-3's `DoD:65` grep stays at `20`.

### serve-3 — Still open

- `Session.version` — **decided 2026-10-04: keep the build constant**
  (`coding/version.Version`). Closes open question 3 in the slice spec. Alternatives and
  evidence below.
- The tail read's byte cap wins silently on large entries — `limit=20` returns 15 when
  entries are 1 MiB. The spec sanctions "whichever bound is reached first"; whether the
  client should be told is unanswered.
- The spec's DoD:65 grep names `serve.go` for `writeJSON`, which now lives in
  `serve_handlers.go`. The property holds; the gate text is stale.
- `docs/plans/2026-10-04-serve-3.md` is 2112 lines and was revised seven times (§12 is the
  merge-blocking round). Read it before touching this code, as serve-2's was.

### serve-3 — `Session.version`, decided (open question 3 closed)

**Decision: keep `coding/version.Version` (the build constant).** The plan (plan:181-183)
says "derive both deterministically from the session ID"; the implementation ships the
constant instead. Decided 2026-10-04 by a four-dispatch fan. Three dispatches owned this
question: two returned a firm `DECIDED: keep the constant` on independent reasoning, and the
third — the client-contract angle — returned `UNVERIFIED` because it could not settle the
question from inside this repo, while naming the constant the **zero-risk option rather
than a proven-correct one**. That is the honest shape of the convergence: the code decides
this, the client contract only fails to object. The plan text must be amended to say "emit
the build constant", or the contradiction regenerates on the next read.

The reasoning that actually decides it: **the plan states a purpose, and names a means that
is not required to achieve it.** Its purpose is stability "across reconnects"
(plan:181-183); the constant is a `const` (`coding/version.go:6`), so it is stable across
reconnects by construction, as is any pure function of the session ID. The stated rationale
therefore does not discriminate between the two options — and the remaining ground is
honesty, which favours the constant.

Supporting findings, each with evidence:

- The plan **never says the client compares `Session.version`** against anything, and never
  states its **Swift type**. Exhaustive `grep -i version` over the plan returns exactly
  three hits: `:120`, `:175`, `:181`. Every Swift citation in the plan (`:149`, `:187`,
  `:199`) is about something else. So the contract's only stated requirement is *presence* —
  "tolerant of absent optionals and of unknown keys, but not of absent requireds"
  (plan:167-168) — and the constant satisfies it.
- `Session.version` and `/global/health`'s `version` (plan:120) are two fields in two
  different client structs, and the health one is `String?` while the Session one is
  non-optional. Emitting the same constant in both is what keeps the key meaning one thing
  across the API; emitting `header.Version` would put a build string and a format `int`
  under one JSON name.
- **`header.Version` is a bad candidate, and the slice spec's stated reason for dismissing
  it does not hold.** The spec says it "is 0 on a session written before the field existed"
  (`docs/plans/2026-10-04-serve-3.md:89`); no fixture in the repo omits the field, and git
  history is a single squashed `9396b05 Initial commit`, so the claim is unsubstantiated
  rather than refuted. What *is* verified kills the option anyway: the field is
  `Version int` with `omitempty` (`internal/codingagent/session.go:36`), it has **zero
  readers** anywhere in the tree, the loader validates only `Type == "session"` and never
  checks it (`session_manager.go:140-145`), and its doc comment — "Bumping this requires a
  migration step in `session_resume.go`" (`session.go:24`) — is **false**: that file
  contains zero occurrences of `ersion`. Promoting a write-only, unenforced internal
  counter onto the public wire would make it the only value wopr emits that wopr cannot
  itself interpret. It would also emit a JSON number where the plan describes a version
  string.

Alternatives, with costs: **derive from the session ID** (the plan's literal text) — costs a
derivation function and a per-session decision inside a 100-goroutine list fan-out
(`serve_dto.go:117`), and produces a value with **no meaning** in a field the client
requires: the same defect plan:183-184 forbids for zeros-as-totals. One line of code is the
only reason it is tempting. **`header.Version`** — the findings above. **Omit the field** —
highest downside for zero upside: `serve_dto.go:22` has no `omitempty`, so it is always
present today, and removing it puts the whole `GET /session` decode at risk rather than one
session.

Two things recorded because they are unpaid costs of the decision, both landing in serve-3's
fix-and-merge commit: `serve_dto_test.go:26` hardcodes the literal `"0.2.1"` in its
round-trip fixture, so the next version bump fails a green test — `AGENTS.md` forbids
change-detector shapes, and it should read `version.Version`; and nothing pins the constant
in that test beyond the literal, so the field is not actually asserted on.

Unverified, and it is what would reopen this: no Swift source exists in this repo, so
whether the client *reads* the field beyond decoding is unknown. If it compares
`Session.version` across sessions or invalidates on change, a constant silently disables
that — check `grapeot/opencode_ios_client` at commit `2d0e7bd` (plan:368-371). Note also
that the client *build* is itself unresolved: three candidate apps exist, and every citation
above is conditional on which one the plan was diffed against.

### serve-7 — Permission approval substrate

- **Status:** DONE — closed by decision 2026-10-06, nothing implemented. Policy is
  **all tools open by default** in served mode: no approval gate, no `permission.asked`,
  the two `GET/POST /permission*` routes stay 501. Reopen only if a concrete need for a
  thumb appears; the `Blocked on` policy question below is then the spec's first input.
- **Why:** `GET /permission` and `POST /session/{id}/permissions/{id}` are served as 501
  stubs by serve-5 (see serve-3's scope-reduction entry above). The endpoints are in the
  client contract (plan:142-143), the app renders a permission prompt, and wopr has no
  answer for any of it.
- **Blocked on:** what actually requires approval. wopr has no approval policy anywhere —
  Settings carries only `ShellPath` and `CommandPrefix` (`internal/codingagent/settings.go:77-80`).
  Until a human decides what needs a thumb and what does not, there is nothing to build
  toward, and inventing a policy is a product decision, not an implementation.
- **Files (when unblocked):** an approval queue in `coding/host`; the first *blocking* hook
  in the tree, which lands in the agent loop's serial prep phase
  (`agent/tool_execution.go:103-121`) and so stalls every tool in the batch rather than just
  the one awaiting a reply — all five existing hook users return synchronously; the
  `permission.asked` / `permission.replied` SSE events (plan:157), which have no origin in
  Go code and belong to serve-4's mapping; the two DTOs the plan never specifies.
- **Notes:** the blocker for a naive implementation is replay. The client has no
  `Last-Event-ID` and no replay (plan:162-163), so a permission asked while the phone was
  out of range is unrecoverable. That makes this a liveness protocol needing its own
  persistence story, and it is why deferring it was the cheap answer rather than a
  compromise. A CHANGELOG line is owed whenever it lands: it changes what the model can do
  in every mode, the same reason serve-5 carries one.

### serve-4 — Prompt path and SSE

- **Status:** DONE — shipped on `wave-serve-4` as four commits (prompt path, SSE, defect
  fixes, corrections)
- **Depends on:** serve-2, serve-3
- **Why:** The slice that proves the seam. Everything before it is plumbing; this is
  whether a phone can hold a real conversation with a session on another machine.
- **Files:** `internal/codingagent/serve/` prompt handlers, the SSE stream, the
  opencode event mapping.
  (`coding/host`'s per-session run rule shipped with **serve-1**, not here — this item
  double-claimed it. Do not rebuild it; `coding/host/host.go:97-104` already refuses a
  second concurrent prompt with `ErrSessionBusy`.)
- **Gates:** `prompt_async` answers 204, `abort` answers 200, and the SSE stream carries
  all 17 bus event types with the per-connection byte bound and the terminal drop event.
- **Notes:** Do not start SSE before a prompt streams correctly — message topology is the
  real risk here, since opencode's parts address a message that also addresses the
  assistant reply while wopr's transcript is an ordered entry list. Size the
  per-connection buffer in **bytes**, not events: one session entry can exceed 1 MiB
  (`internal/codingagent/session_large_line_test.go:32` pins a 17 MiB entry), so an
  event-count cap is not a bound. Serialize each event once and share the bytes.

#### serve-4 — The gate, restated because it was not meetable as written

`Gates:` above said the SSE stream "carries all 17 bus event types". **9 of the 17 have no
trigger in wopr**, so that sentence was never satisfiable and shipping against it would have
meant either a fake gate or placeholder emissions. Restated:

- **the mapping covers all 17 names; 8 have a live trigger in serve-4; the other 9 are
  justified by name.** No reservation table ships — a string table with no reader is dead code
  and `unused` is enabled (`.golangci.yml:18`); the §Q3 prose in
  `docs/plans/2026-10-04-serve-4.md` is the record.
- The nine: `todo.updated`, `question.asked`, `question.replied`, `question.rejected` and
  `permission.replied` are serve-5 and serve-7. `message.removed`, `message.part.removed` and
  `session.deleted` are a wopr concept with nothing that fires — `AppendContextEdit` emits no
  event, and `DELETE /session/{id}` is a 501 stub.
- The other two original gates are met literally: `prompt_async` answers 204, `abort` answers
  200, and the per-connection bound is in bytes with the terminal drop event.

#### serve-4 — What landed

- `POST /session/{id}/prompt_async` → 204, `POST /session/{id}/message` → 200 `{info, parts}`,
  `POST /session/{id}/abort` → 200, `GET /global/event` → SSE. 404 / 409 / 400 / 413 all
  specified and covered. `make check` green end to end, `go fix -diff` empty, `-race` green on
  `coding/host` and `serve` (the only race coverage those packages get — `make check`'s
  `test-race` pins `./internal/codingagent` only).
- **The byte bound is real and measured:** 33,498,094 queued bytes against a 33,554,432 bound
  with four stalled readers over 8,000 entries; an unbounded queue would have held ~500 MiB per
  stream. `Publish` marshals `ev.Data` once and queues it **beside `ev.Type`** — a subscriber
  holding only payload bytes has no event name and hands every frame to the client untyped.
  **This was a BLOCKER caught in the spec fan and it is the load-bearing detail of the design.**
- **Message topology, the item's named risk, works — on live data.** The transcript entry id does
  not exist while deltas stream, so deltas use a host-minted provisional message id and
  `MessageEndEvent` reconciles with the real one in a burst whose part ids match
  `GET /session/{id}/message` byte for byte. Verified by curl against a live listener, not only
  by test.
- **`appendSessionLine` separator fix shipped here, retiring serve-3 carried-debt item 1.**
  Applying the client's `model` field reaches that function, so serve-4 was in the blast radius
  whether it liked it or not; leaving a known data-loss path unfixed to protect a diff would
  have been the worse call.
- `partID` is one helper used by both the stream and the transcript, with the index enumerated
  over the **full block slice before any filtering** so it equals `ai.*Event.ContentIndex` even
  when a block yields no part. This retires a live serve-3 defect: `entriesToMessages` iterated
  indexless, so two text blocks in one message produced two parts with the same id.
- `messageDTO` became `{info, parts}` — opencode's `MessageWithParts`, which is what the client
  decodes. The flat half serve-3 shipped was speculative.
- `(*Session).MessageEntryID` exists because `TurnEndEvent.MessageEntryID` **never reaches
  `Session.Events()`**: `agent.emit` sends the pre-hook value, so the id was assigned in one
  place and thrown away.

#### serve-4 — Carried debt, recorded rather than fixed

1. **The `r.Context()` trap is closed and pinned; the abort reservation window is not.** The
   owned cancel is load-bearing for `prompt_async` and a test proves it (replacing
   `context.WithoutCancel` with the bare context turns the test red). But
   `TestAbortStopsARunningPrompt` stayed **green** when that cancel was deleted — by the time the
   test observes streaming, `beginAgentRun` has already stored its own. The window between
   reservation and `beginAgentRun` is not deterministically reachable from an HTTP test without
   a hook in `coding/`. The cancel is correct; it is simply not independently pinned.
2. **The byte-bounded queue has two unpinned properties.** (a) `pop` clearing the popped slot is
   what makes a *draining* connection release its memory — measured 31.0 MiB retained → 0.0 —
   but no committed test sees it, because a `HeapAlloc`-after-drain assertion is GC-timing
   dependent and would be flaky. (b) `Push` signalling under the mutex, which removes a stale
   wakeup token, has no test either: `TestDrainedBurstDoesNotFakeADeadline` is entirely
   single-goroutine. Both are correct today; both are undefended. Pin them when a deterministic
   oracle exists.
3. **`agent/agent.go:1296` (`agentAssistantMessage`) reads the stream builder's live message.**
   Pre-existing, and the SSE path does not reach it. The real hazard is one line wide:
   `snapshotAssistantEvent`'s `default:` arm returns events unsnapshotted, so any future
   `AssistantMessageEvent` type the mapper handles would arrive live. It is the same class as the
   `session_recovery.go` race this slice fixed. A `coding/` change, out of scope here.
4. **`ai/messages.go:222` — `cloneAssistantContent` asserts
   `cloneJSONValue(...).(map[string]any)` with no `, ok`.** A `ToolCall` with a nil `Arguments`
   map panics inside `snapshotAssistantEvent`. Unreachable from the builder today
   (`stream_builder.go:189` always sets `Arguments`), and pre-existing.
5. **`type:"retry"` is unreachable on the pull path.** `coding.Session` exposes only
   `SetAutoRetryEnabled` and `AbortRetry`, so `GET /session/status` cannot report it; `retry` is
   emitted on the SSE stream only. `SessionStatus.type` must be one of three values, so omitting
   one is valid, not a decode failure — but the two surfaces disagree.

#### serve-4 — A verification finding that was wrong, and how it was caught

A fan claimed `toolPartID` raced on the stream builder's live `*ai.AssistantMessage`. The
implementer rejected it on the grounds that `AssistantMessageEventStream.Push` calls
`snapshotAssistantEvent` on every event, and a second fan **confirmed the implementer, not the
finding**, two ways: by the code path (`ai/event_stream.go:44`, before the mutex, before
queueing) and empirically (a throwaway test mutated the producer's live object after the push
and the consumer still saw the original id). The first fan read `ai/stream_builder.go`, which
does push the live pointer, and stopped one layer short. **A verifier's rejection is as much a
hypothesis as its finding.**

#### serve-4 — What the next session should not repeat

Four rounds of verification, and the gates found real defects each time:

1. `POST /message` answered **500 "run produced no persisted assistant message"** for a busy
   session — `PromptContentWait` ran in a goroutine, so `ErrSessionBusy` arrived with an empty
   entry id and the empty-id branch fired before the error was read. A test existed for
   `prompt_async`'s 409 and none for `/message`'s.
2. `pop` advanced the slice header without clearing the slot, so a drained burst kept every
   payload: **31 MiB retained after draining 31 MiB**, against a bound whose whole purpose is that
   a draining connection costs nothing.
3. `Push` signalled after unlocking, leaving a stale token; every `Next` returned a spurious
   deadline, which the SSE handler maps to a heartbeat. **The 15s heartbeat pacing was dead** and a
   busy stream heartbeated after every drained burst.
4. The `appendSessionLine` fix shipped with **no test at all** — reverting it made nothing red
   anywhere, on a data-loss path `AGENTS.md` explicitly requires a test for.
5. `Host.Start` called the event forwarder **synchronously**, so `POST /session` hung for 60s.
   No unit test caught it; curl did, immediately.

The pattern: **every one of these passed the whole suite.** The curl-against-a-live-listener
step is what found #5, and the break-the-code-underneath-the-test step is what found #1 and #4.

### serve-5 — Todos, questions, and the file allowlist

- **Status:** DONE — shipped on `wave-serve-5` as `3b97e99`, merged into `main` as `42b2c5a`.
  Fix round from the verification fan closed one blocker (a rejected `todowrite` broadcast
  an empty `todo.updated`) and two warnings; all six gates green, `make check` passed.
- **Depends on:** serve-4
- **Why:** The client's UI genuinely consumes todos and the agent's ask-the-user tool,
  so without these the phone is a read-only viewer of a conversation it cannot answer.
- **Files:** a `todowrite` tool; `askuser.Asker` wiring; `/todo`, `/question*`,
  `/file/content` handlers and the resolved-path allowlist in `internal/codingagent/serve`.
- **Gates:** a behavioral test for `todowrite`, an ask/reply round trip through the
  question endpoints, and allowlist tests including a symlink that points outside the
  project root.
- **Notes:** The client reads todos from either the session todo list or `todos` on the
  part's own metadata (`ToolPartView.swift:103`), so a `todowrite` result carrying todos
  in its metadata satisfies both surfaces. `askuser.Asker` already exists and blocks on
  `ask(ctx, q)`; RPC omits it deliberately (`coding/session_ask.go:12`) because a headless
  client should not pay request tokens for a question nobody answers — a phone is the
  opposite case. This changes what the model can do in **every** mode, so it needs a
  CHANGELOG line. Snapshot-based revert is a separate plan; 501 until then.

### serve-6 — Docs and threat model

- **Status:** DONE — shipped on `wave-serve-6` as `8f63af8` plus a fix commit
  `64f7c16` (the diff verifier caught `POST /session` documented as 409 when the
  shipped command actually answers 500, and the six 501 stubs called "mutating"
  though two are GET), merged into `main` as `7da8dfa`. Seven docs files, no Go
  change. `docs-drift` green; `build`/`vet`/`lint`/`go fix`/grouped/race all green
  in the worktree and re-run on merged `main`. `make check` stops at `test-prereqs`
  (tmux absent, `Makefile:110-111`), so `test-integration` was not run.
- **Depends on:** serve-4, serve-5
- **Why:** An unauthenticated server that runs tools and reads files must say so where
  the person binding the port will actually read it.
- **Files:** `docs/site/docs/server.md` (new), `docs/threat-model.md`,
  `docs/site/docs/settings.md`, `docs/architecture.md` package map, `README.md`.
- **Gates:** `tests/docs-drift` via `make check`.
- **Notes:** `server.md` states the client commit the endpoint table was derived from,
  because the app is a third-party build with no SLA. The threat-model section says
  plainly that a served session is a remote shell for anyone who can reach the port, and
  that auth-less LAN serving rests on the path allowlist.

---

## Follow-ups surfaced by serve-5

### serve-allowlist-toctou — Close the /file/content resolve-to-open race

- **Status:** DONE — shipped on `wave-serve-allowlist-toctou` as `d8ca6a1`, merged into
  `main` as `b3890a1`.
- **Depends on:** serve-5
- **Why:** `fileContent` resolves the requested path through `resolveInRoot`
  (`Lstat` + `EvalSymlinks`) and then calls `os.Open` on the resulting string. A
  component inside the root swapped for a symlink pointing outside between the two is
  followed by the kernel, defeating the path allowlist that auth-less LAN serving rests
  on. It needs a writer with access to the served root, which is why serve-5 ships the
  static-symlink gate and this is a follow-up.
- **Files:** `internal/codingagent/serve/serve_handlers.go` (`fileContent`),
  `internal/codingagent/serve/serve_dto.go` (`resolveInRoot`).
- **Gates:** a test that swaps a path component for a symlink between resolution and
  read and asserts a 403, or a traversal that cannot follow a swapped link.
- **Notes:** A full fix is per-component `O_NOFOLLOW` (`openat`-style) traversal, or
  opening the file and re-checking the open descriptor against a freshly resolved path
  with `os.SameFile`. serve-5's DoD 8 covers the static symlink only.
- **Shipped (`b3890a1`):** the open runs through `os.OpenInRoot` on the already-canonical
  relative path, so the kernel walk is confined to the root and a swapped component is
  refused, not followed. The pre-open `os.Stat` is kept so a statically-present
  FIFO/device is answered 400 before any `openat`; open failures are reclassified by a
  fresh `resolveInRoot` (403/404, else 500). A `beforeOpen` test hook lets the gate swap a
  component in the resolve->open window; the test covers a leaf-file and an
  intermediate-directory swap and serves the outside content if the confinement is
  removed. Known residual: the pre-open `os.Stat` still follows a swapped symlink, so an
  outside file's existence (404) and size (413 body) can be observed — metadata only, no
  content. `listFiles` keeps the same class of gap; see `serve-file-list-race`.

### serve-ci-race-serve — `make check` does not race-test `internal/codingagent/serve`

- **Status:** DONE — shipped on `wave-serve-ci-race-serve` as `6cf2a46`, merged into
  `main` as `f48c353`.
- **Depends on:** serve-5
- **Why:** `automation/ci/test-race.sh:19` pins `pkg=./internal/codingagent` with a fixed
  `-run` filter, so `internal/codingagent/serve` gets no `-race` coverage from
  `make check`. serve-5 added a store written by a tool goroutine and read by HTTP
  handlers, which is exactly that gap; its spec added a separate `-race` line that
  nothing enforces.
- **Files:** `automation/ci/test-race.sh`; `Makefile` if the grouping lives there.
- **Gates:** `make check` runs the serve tests under `-race` and fails on a seeded race.
- **Notes:** Put it in the race script, not a doc, so the next serve slice inherits it.
- **Shipped (`f48c353`):** `automation/ci/test-race.sh` now runs
  `go test -race -count=1 ./internal/codingagent/serve` — whole package, no `-run` filter, so
  every current and future serve test inherits coverage — inserted between the existing TUI
  race block and the goroutineleak profile; line 2's summary was broadened to name serve.
  `make check`'s `test-race` step now compiles and runs the serve package under `-race`
  (`ok .../serve`). Proven to fail: an untracked seeded `TestSeedRace` in `package serve`
  produced `WARNING: DATA RACE` and a non-zero script exit; removed, the script is green.
  Re-verified `make test-race` green on `main` after the merge. `make check` itself still
  cannot complete here — `tmux` absent, it stops at `test-prereqs` (`Makefile:110-111`); every
  other constituent was run directly and passed (`go build ./...`, `go vet ./...`, `make lint`
  0 issues, the grouped unit suite, `go fix -diff` empty, docs-drift).
- **Surfaced here:** `coding-host-ci-race` (same gap for `coding/host`, which serve-4 only
  race-tested manually) and `serve-ci-race-doc-drift` (the TUI-only descriptions left elsewhere).

### serve-5-regressions — Pin the two serve-5 fix-round branches

- **Status:** DONE — shipped on `wave-serve-5-regressions` as `60044ef`, merged into
  `main` as `4394754`.
- **Depends on:** serve-5
- **Why:** Two fix-round branches are reasoned-correct but unpinned: a rejected
  `todowrite` must not broadcast an empty `todo.updated`, and the `ask` abort-vs-reply
  race must honour a reply that already answered 200. Reverting either guard would make
  nothing red today.
- **Files:** `internal/codingagent/serve/serve_sse.go` + `serve_test.go`;
  `internal/codingagent/serve/serve_question.go` + `serve_test.go`.
- **Gates:** one test per branch that fails when the guard is removed.
- **Notes:** The event case needs a `todowrite` fixture that `Parse` rejects, so a
  `Failed todowrite` trigger in `ai/test_faux.go` comes with it. The abort path needs a
  deterministic interleaving of a reply and an abort.
- **Shipped (`4394754`):** both guards pinned, production untouched. `serve_test.go`
  gains `TestRejectedTodowriteDoesNotBroadcastTodoUpdated` (a duplicate-id
  `"Run: failed todowrite"` fixture fails `Parse`, the mapper emits an error tool
  part, and no `todo.updated` may appear before the idle stop frame — a
  protocol-level control replaces the error-string coupling) and
  `TestAskHonoursAReplyThatClaimedTheEntryBeforeAbort` (single-shot deterministic
  `testing/synctest`: the entry is claimed, the context cancelled while the answers
  slot is empty so `ctx.Done` is the only ready select case, and the reply's answer
  must survive). Both proven red: removing the `todos != nil` guard fails on
  `todo.updated {todos:[]}`; removing the `<-pending.answers` fallback fails with
  `err=context canceled`. Gates green: build, vet, `-race` serve+ai, lint (isolated
  cache), `test-race.sh`, grouped fast+cli, `go fix -diff` empty, docs-drift.
  `test-integration` not run (tmux absent).

### serve-file-list-race — `GET /file` resolves then `ReadDir`s the string

- **Status:** DONE — shipped on `wave-serve-file-list-race` as `8f6f37b`, merged into
  `main` as `d415ec2`.
- **Depends on:** serve-5, serve-allowlist-toctou
- **Why:** `listFiles` (`serve_handlers.go:468-515`) resolves the requested directory
  through `resolveInRoot` and then calls `os.ReadDir(resolved)` on the string. A component
  swapped for an outside-pointing symlink between the two is followed, so a writer with
  access to the served root can list an outside directory's entry names. The
  serve-allowlist-toctou wave fixed only `/file/content`; this is the same race on the
  listing path — names rather than content, but the same allowlist.
- **Files:** `internal/codingagent/serve/serve_handlers.go` (`listFiles`).
- **Gates:** a test that swaps a directory component for an outside symlink between
  resolve and list, asserting 403 and no outside entry names; the `beforeOpen` hook
  pattern from serve-allowlist-toctou transfers.
- **Notes:** `os.OpenInRoot(root, rel).ReadDir(-1)` confines the walk the same way the
  content fix does; keep the pre-open `os.Stat` so a path that resolves to a file is still
  400.
- **Shipped (`d415ec2`):** `listFiles` is a nil-passing wrapper over a new
  `serveListFiles(w, r, beforeOpen)`; the walk goes through `os.OpenInRoot(root, rel)` and
  `ReadDir(-1)`, with open failures classified by a fresh `resolveInRoot` (403/404, else
  500). `(*os.File).ReadDir(-1)` returns directory order, so entries are sorted by name to
  reproduce `os.ReadDir`'s documented order and keep the `maxFileEntries` survivor set
  unchanged. The pre-open `os.Stat` and the `!info.IsDir()` → 400 branch are kept. The
  `beforeOpen` hook drives two swap subtests (requested directory and intermediate
  directory); the test serves the outside entries when the confinement is removed. Same
  residual as the content path — the pre-open `os.Stat` still follows a swapped symlink,
  so an outside directory's entry *names* (not contents) could be observed if it reaches
  the stat first; the confinement closes the listing itself.

### serve-file-list-directory-param — `/file` ignores the `directory` query

- **Status:** FOLDED into `session-directory` on 2026-10-06 — no longer a
  separate dispatchable item. Decided: reject a foreign `?directory=` with 400,
  matching the four sibling handlers (the API stays a validation against the
  process root; see `session-directory` decision 4). Delivered by
  `session-directory` slice 2 (`9062a52`): `serveListFiles` gains
  `directoryParam`; `server.md`'s `/file` row lists `directory`.
- **Depends on:** serve-file-list-race
- **Why:** `listFiles` reads only `path` and never calls `directoryParam`
  (`serve_handlers.go:64-77`), while every sibling handler including `serveFileContent`
  does. `/file?directory=<other>` lists the session CWD instead of validating or rejecting
  the foreign directory.
- **Files:** `internal/codingagent/serve/serve_handlers.go` (`listFiles`).
- **Gates:** delivered by `session-directory` — `serveListFiles` calls `directoryParam`
  and `/file?directory=<foreign>` answers 400 like the sibling handlers (plan slice 2).
- **Notes:** Left out of serve-file-list-race to keep that slice to the security fix.
  Semantics decided 2026-10-06 (reject with 400); delivered by `session-directory`.

### coding-host-ci-race — `make check` does not race-test `coding/host`

- **Status:** **DONE** — shipped on `wave-coding-host-ci-race` as `d25866e`,
  merged into `main` (`--no-ff`) as `c6d2af7`. 2026-10-06.
- **Depends on:** serve-ci-race-serve
- **Why:** `serve-ci-race-serve` added `internal/codingagent/serve` to
  `automation/ci/test-race.sh`, but `coding/host` — the multi-session supervisor and typed
  event bus from serve-1, with a concurrent session registry and a per-session
  `sync.Mutex.TryLock` — still gets no `-race` coverage from `make check`. serve-4 ran it under
  `-race` manually only.
- **Files:** `automation/ci/test-race.sh`; `coding/host` tests.
- **Gates:** the serve race block's twin for `coding/host`, plus the same seeded-race proof
  (a race in `coding/host` makes the script exit non-zero).
- **Notes:** Same shape as the serve fix — whole package, `-count=1`, no `-run` filter.

### serve-ci-race-doc-drift — reconcile the TUI-only `test-race` descriptions

- **Status:** DONE — shipped on `wave-serve-ci-race-doc-drift` as `9975d23`, merged into `main` as `0d55c16`
- **Depends on:** serve-ci-race-serve
- **Why:** `serve-ci-race-serve` broadened `automation/ci/test-race.sh:2` to name serve, but
  the explanatory block (`test-race.sh:3-16`), the Makefile comment and help
  (`Makefile:131-135`), and `automation/README.md:48` still describe `test-race` as the TUI
  concurrency gate only. They now under-describe what the target runs.
- **Files:** `automation/ci/test-race.sh`, `Makefile`, `automation/README.md`.
- **Gates:** the descriptions name both the TUI and the serve/backend packages; no behavior
  change (the script's commands stay as they are).
- **Notes:** Cosmetic. The `serve-ci-race-serve` spec deliberately deferred it to keep that
  slice to the gate fix.

### serve-ci-host-race-echo — `test-race.sh` host echo overstates the cross-race

- **Status:** DONE — shipped on `wave-serve-ci-host-race-echo` as `369fa93`,
  merged into `main` as `e1f4ac2` (`--no-ff`), 2026-10-06.
- **Depends on:** serve-ci-race-doc-drift
- **Why:** `automation/ci/test-race.sh:37`'s echo says "host: multi-session supervisor vs
  typed event bus", implying a supervisor↔bus cross-race that `coding/host`'s tests do not
  create — no test installs a `Translator`, so `forwardEvents` drops events. The command is
  correct (it does race the package); only the prose promises a race the tests never
  exercise.
- **Files:** `automation/ci/test-race.sh` (the `host` echo line, ~37).
- **Gates:** the echo names what the host tests actually race, or a host test is added that
  installs a `Translator` so the promised supervisor↔bus race is real; `tests/docs-drift` via
  `make check`.
- **Notes:** Surfaced by `serve-ci-race-doc-drift`, which was forbidden from touching script
  commands. Cosmetic unless the second gate option is taken, in which case it is a real
  coverage gap in the same class as `coding-host-ci-race`.
- **Landed:** Took Option A — corrected the prose, no test added. `automation/ci/test-race.sh:37`
  now reads `host: concurrent session prompts + racing subscriber close`, and the block-header
  comment at `:23-24` names the same two races so the file is internally consistent. No command
  changed (the `go test -race -count=1 ./coding/host` line is byte-identical). Chose A over B on
  `AGENTS.md`'s no-new-tests default and the `docs-drift-docs-json` precedent: the defect is
  cosmetic and no money/data/safety cost attaches. Option B's marginal value is smaller than the
  original note implied — no `coding/host` test installs a `Translator`, but the serve gate
  (`test-race.sh:35`) does exercise that forward path, because `serve_test.go:168` calls
  `serve.New` which sets `h.Translate` (`serve.go:44`) and then drives prompts. Spec
  `docs/plans/2026-10-06-serve-ci-host-race-echo.md`. Verified by a 2-angle spec fan
  (runtime-seam, leanness/scope) whose Evidence-section findings were corrected before
  dispatch, and a 2-angle diff fan (scope-fit, gate-integrity), both CLEAN. Gates:
  `bash -n`, `bash automation/ci/test-race.sh` (new echo printed, green), `go build ./...`,
  `go vet ./...`, `go tool golangci-lint run` (0 issues), `go fix -diff` (clean),
  `go test ./tests/docs-drift/ -count=1` (ok), `go test ./... -count=1` (936 passed, 58
  packages). `make check` still stops at `test-prereqs` (tmux absent); `test-integration`
  likewise untested (tmux).

---

## Follow-ups surfaced by serve-6

### serve-cli-doc — `cli.md` never documents the `wopr serve` verb

- **Status:** DONE (`wave-serve-cli-doc`, commit `8186386`, merged `d2842ec`)
- **Depends on:** serve-6
- **Why:** `wopr serve` is a live CLI verb with `--port`/`--hostname` flags
  (`cmd/wopr/serve.go`), but `docs/site/docs/cli.md`'s command table has no row for it
  and a grep for "serve" in that page is empty. A reader who wants the server finds it
  only via README or `server.md`; the CLI reference is incomplete.
- **Files:** `docs/site/docs/cli.md`.
- **Gates:** `tests/docs-drift` via `make check`; the new row names the real flags and
  default behavior.
- **Notes:** Surfaced by serve-6, which was scope-locked to its five named files plus the
  index/docs.json needed to publish `server.md`. `server.md` is the contract; `cli.md`
  should point at it rather than duplicate the endpoint table.
- **Landed:** `docs/site/docs/cli.md` gains a `wopr serve [--port N] [--hostname H]` row in
  the Generic subcommands table, naming both flags, `server.port` default `4096` and
  `server.hostname` default `127.0.0.1`, and linking `[server](server.md)` instead of
  duplicating the endpoint table. Spec `docs/plans/2026-10-06-serve-cli-doc.md`. Verified by
  a 2-angle spec fan and a 2-angle diff fan (runtime-seam, scope-fit), both CLEAN. Flags and
  defaults re-derived from `cmd/wopr/serve.go:35-70` and
  `internal/codingagent/settings.go:257-258`. Gates: `go build ./...`, `go vet ./...`,
  `go test ./tests/docs-drift/ -count=1` (6 passed), `go tool golangci-lint run` (0 issues),
  `go fix -diff` clean, `go test ./... -count=1` (936 passed, 58 packages). `make check`
  still stops at `test-prereqs` (tmux absent).

### docs-drift-docs-json — `development.md` claims docs-drift checks `docs.json`

- **Status:** DONE (`wave-docs-drift-docs-json`, commit `2625071`, merged `494fa4d`)
- **Depends on:** none
- **Why:** `docs/site/docs/development.md:76` says "`docs/site/docs/docs.json` classifies
  and orders every page, and `make docs-drift` checks it." Neither test under
  `tests/docs-drift` opens `docs.json`; `make docs-drift` is only
  `go test ./tests/docs-drift/`. The claim overstates the gate, and a page added to the
  bundle without a `docs.json` entry passes green.
- **Files:** either `docs/site/docs/development.md` (correct the claim) or
  `tests/docs-drift` (add the check the page promises).
- **Gates:** `tests/docs-drift` via `make check`.
- **Notes:** Surfaced by serve-6, which added the `server.md` provenance and navigation
  entries by hand because no gate would have caught their absence.
- **Landed:** Took option A — corrected the claim rather than adding a gate. Deleted
  "`, and `make docs-drift` checks it`" from `development.md:76`; the page now reads
  "`docs/site/docs/docs.json` classifies and orders every page." Chosen because the repo's
  own test rule (`development.md:68`, `AGENTS.md`) bars new tests without a
  money/data/safety cost, `docs.json` is not embedded (`embed.go:11` is `//go:embed *.md`)
  and no code reads it, the page-unreachable harm is already gated by
  `TestEveryPageIsReachableFromTheIndex`, and adding a check would have made stale the
  `Makefile:158-160` gate comment. Spec `docs/plans/2026-10-06-docs-drift-docs-json.md`.
  Verified by a 3-angle spec fan (leanness, runtime-seam, completeness) and a 2-angle
  diff fan (scope-fit, claim-accuracy), all CLEAN; claim-accuracy recomputed
  provenance == navigation == 42 `.md` pages. Gates: `go build ./...`, `go vet ./...`,
  `go test ./tests/docs-drift/ -count=1` (6 passed), `make lint` (0 issues),
  `go fix -diff ./...` clean, on both the wave branch and merged `main`. `make check`
  still stops at `test-prereqs` (tmux absent, `Makefile:110-111`).

### docs-drift-cross-dir-links — docs-drift cannot see links outside the bundle

- **Status:** DONE (`wave-docs-drift-cross-dir-links`, commit `40d7c8f`)
- **Depends on:** none
- **Why:** `TestInternalLinksResolve`'s `docLink` regex
  (`\]\(([A-Za-z0-9_.-]+\.md)`) matches a single path segment, so a link like
  `../../threat-model.md` is invisible to it. serve-6 added two such links from
  `server.md` to `docs/threat-model.md`; they are correct today, but a future rename or
  deletion of `docs/threat-model.md` would break them with docs-drift still green.
- **Files:** `tests/docs-drift/docs_drift_test.go`.
- **Gates:** `tests/docs-drift` via `make check`; the check resolves multi-segment
  relative links against the repo root, and a deliberately broken one fails.
- **Notes:** Surfaced by the serve-6 diff verifier. Low blast radius; the two links are
  manually verified now.
- **Landed:** `docLink` widened to capture multi-segment relative paths; `TestInternalLinksResolve`
  resolves a `target` containing `/` from the linking page's directory
  (`filepath.Join(contentDir, filepath.Dir(name), target)`) and rejects a missing file with
  `os.Stat`, naming the page. Red-proof: a broken `../../threat-model.md` in `server.md`
  fails the gate. Verified by a two-angle diff fan.

### docs-drift-link-syntax-ceiling — docs-drift still misses links in non-plain Markdown forms

- **Status:** DONE (`wave-docs-drift-link-syntax-ceiling`, commit `ac00a15`, merged `1d41da1` `--no-ff`)
- **Depends on:** none
- **Why:** after `docs-drift-cross-dir-links`, `docLink` still matches only the plain form
  `](path.md[#frag])`. CommonMark-valid destinations with a title (`](page.md "Title")`),
  spaces or percent-encoding (`](my file.md)`, `](my%20file.md)`), angle brackets
  (`](<page.md>)`), and uppercase `.MD` are not matched at all, so a broken link in any of
  those forms passes the gate silently. Pre-existing; the cross-dir fix only widened the
  path-segment alternation.
- **Files:** `tests/docs-drift/docs_drift_test.go`.
- **Gates:** `tests/docs-drift`; a deliberately broken titled/encoded link fails.
- **Notes:** Surfaced by the cross-dir wave's false-green verifier. No such link exists in the
  bundle today, so the fix can be verified against a temporary one.
- **Landed:** `docLink` widened to the segment-structured pattern
  `\]\([ \t]*<?([A-Za-z0-9_.% -]+(?:/[A-Za-z0-9_.% -]+)*\.(?i:md))(?:#[^)]*)?>?(?:[ \t]+"[^"\n]*")?[ \t]*\)`
  (`tests/docs-drift/docs_drift_test.go:163`): angle-bracketed destination, trailing
  title, literal spaces, `%`-encoding, and upper-case `.MD` now match; group 1 keeps its
  old meaning so neither consumer changed. Segment structure preserved the old rejection
  of `//host`, `/.md`, `a//b.md` after the first cut (flat class) over-matched. Red-proof:
  a temporary line in `index.md` with one broken link per form fails
  `TestInternalLinksResolve` five times; removed, green. Spec
  `docs/plans/2026-10-06-docs-drift-link-syntax-ceiling.md`. Verified by a 4-angle spec fan
  (completeness, spec-fidelity, convention-feasibility, runtime-seam) and a 3-angle diff
  fan (scope-fit, gate-integrity, adversarial regex) after the correction; two warnings
  (fragment tail regression, protocol-relative over-match) were fixed before commit, the
  remainder documented as accepted. Accepted ceilings (false-red-only, absent from the
  bundle): a stray `>` after a bare destination (`](foo.md>)`) matches because RE2 cannot
  pair the optional `<`/`>` without a second capture group, and a `%`-encoded external URL
  matches because `%` is needed for the `%20` form and we do not decode. No new test;
  both consumers key `match[1]` unchanged. Gates: `go build ./...`, `go vet ./...`,
  `go test ./tests/docs-drift/ -count=1` (6 passed), `go tool golangci-lint run`
  (0 issues, after `cache clean`), on the branch and re-run on merged `main`.
  `make check` still stops at `test-prereqs` (tmux absent, `Makefile:110-111`).

---

## Open questions blocking nothing yet

- `serve-4`'s manual gate needs the actual iOS app, a laptop-hosted server, and a
  phone. If those are unavailable when that slice lands, the slice ships `UNVERIFIED` on
  that one gate and says so — do not claim it passed.
- Whether `--mode rpc` should gain the question path once `askuser` is wired in
  `serve-5`, given that omitting it is correct for a headless client and wrong for a
  phone.

---

## Product features (outside the serve batch)

### wopr-failover — Fall over to the next model instead of parking a rate-limited run

- **Status:** DONE — shipped on `wave-failover` as `626fb9d`, merged into `main` as `3907f7a` (`--no-ff`)
- **Depends on:** —
- **Why:** A dispatched run on a capped model parks forever: the parent's `task`
  call never returns and the wave silently ends. Today the only remedy is the
  external `opencode-failover` tool (switch a session's model, then abort) plus the
  persona pins — both outside wopr. wopr's own router should own the ladder: on a
  `free_tier_limit`/rate-limit failure, advance to the next configured model in the
  same turn, ratchet past a rung that just refused, and never park a dispatched
  subagent.
- **Files:** `internal/codingagent/router` (model selection; `coding/session_router.go`),
  `ai/` provider streaming retry path, session retry handling; `docs/routing.md`.
- **Gates:** a behavioral test: a simulated 429 / `free_tier_limit` on the active
  model advances to the next and the turn completes without stranding.
- **Notes:** Recorded at the owner's request 2026-10-06, after personas pinned to a
  dead free rung parked every dispatched wave until a server restart.
  `~/.config/opencode/MODELS.md` holds the ladder shape (free-first, paid terminal).
- **Shipped 2026-10-06:** one classifier `ai.IsProviderLimitError` (the old
  `rateLimitPattern` tokens plus `free_tier_limit` in the non-retryable family) and
  one `providerFailure` choke point in `coding/session_task.go` feeding both
  `routeFailover` and `taskHost.Failover`; the subagent failover gate now advances
  read-only children after tool calls (`out.ToolCalls == 0 || req.Type !=
  TypeGeneral`). A routed model that hits `free_tier_limit` advances to the next
  configured candidate in the same turn and rests the failed rung for
  `quotaBackoffMinutes`. Spec: `docs/plans/2026-10-06-wopr-failover.md`.
- **Scope limit (accepted):** this is the routing-active case. With routing off or a
  pinned orchestrator there is no configured ladder and the error is terminal, not a
  hang; an always-on ladder would collide with pin semantics. The literal
  persona-pin incident is therefore not solvable inside wopr without an explicit
  product decision to override pins. See `router-clock-seam` and
  `session-failover-registry-seam` below.

### router-clock-seam — make the failover cooldown assertable end-to-end

- **Status:** DONE — shipped on `wave-router-clock-seam` as `198e2da`+`861cfa5`, merged into `main` as `57a2230` (`--no-ff`)
- **Depends on:** —
- **Why:** `Router.now` is unexported and the only clock seam is `newTestRouter`
  inside package `router`, so a `coding` test cannot observe the 60s-vs-
  `quotaBackoffMinutes` effect at the `taskHost.Failover`/`routeFailover` call sites.
  Surfaced by `wopr-failover`'s verifier fan (Open Q2).
- **Files:** `internal/codingagent/router/router.go` (added exported test-only
  `SetClockForTest`); `coding/session_router_failover_test.go` (the shipped test —
  moved from the originally named `coding/session_retry_classify_test.go`, which is
  classifier-only; see Shipped).
- **Gates:** a `coding` test asserting a `free_tier_limit` failure rests the failed
  spec for `quotaBackoffMinutes`, not 60s.
- **Notes:** optional; the shipped slice pins the classifier at `providerFailure`
  and the cooldown contract in `router_test.go`.
- **Shipped 2026-10-06:** `Router.SetClockForTest(func() time.Time)` (`router.go`)
  is a mutex-guarded test-only clock seam; `New` still defaults to `time.Now`, and no
  production cooldown behavior changed. `TestSessionFailoverRestsTheFailedModelForQuotaBackoff`
  (`coding/session_router_failover_test.go`) drives a `free_tier_limit` failover
  through the real session (`faux-1`→`faux-2`), asserts the post-failover decision is
  `faux-2` and `At` is the fake clock (guards against a vacuous pass), advances 61s,
  and asserts `faux-1` is not re-offered — i.e. it rested `quotaBackoffMinutes`
  (15m), not the generic 60s. RED: compile-red (`SetClockForTest undefined`) before
  the seam; mutation-red (`Next`'s RateLimited cooldown forced to `time.Minute`)
  fails with `faux-1 re-offered after 61s`, reverted. `make check` fully green
  (tmux is installed). Spec: `docs/plans/2026-10-06-router-clock-seam.md`.

### session-failover-registry-seam — register a faux routed model so session failover is testable

- **Status:** DONE — shipped on `wave-session-failover-registry-seam` as `b8e5894`
  (+ `51f25ed` spec-citation fix)
- **Depends on:** —
- **Why:** `routeModel` rejects `test-faux/*` as an unknown model and no test writes
  `router.json`, so session-level same-turn orchestrator failover has no hermetic
  test. Surfaced by the `wopr-failover` spec (Open Q4).
- **Files:** `coding/session_router.go` (`routeModel`), test harness.
- **Gates:** a hermetic session test where an auto-routed orchestrator hits
  `free_tier_limit` and continues on the next model in the same turn.
- **Shipped 2026-10-06:** `NewServices` registers `test-faux/faux-1`/`faux-2` in the
  process registry via `SetProvider` under `WOPR_TEST_FAUX=1` — the same
  dynamic-provider path llama.cpp uses — so `routeModel` accepts them without any
  change to its validation. `ai/test_faux.go` gains a session-scoped
  `"Trigger: free tier limit"` fixture; `coding/session_router_failover_test.go`'s
  `TestSessionFailoverAdvancesToTheNextRoutedModel` drives an auto-routed
  orchestrator that hits `free_tier_limit` on `faux-1` and advances to `faux-2` in
  the same turn (Fallback route event, failed attempt omitted, `failover-ok`). RED
  with the seam stashed, GREEN after; repeatable `-count=3`. Spec:
  `docs/plans/2026-10-06-session-failover-registry-seam.md`.
- **Scope limit (accepted):** the seam is env-gated; with `WOPR_TEST_FAUX` unset,
  `routeModel` still rejects `test-faux/*` and `BuildModel` still errors. Under the
  env, the registration also exposes `test-faux/faux-1|faux-2` on the model-list
  surface (`GetAll`/`RuntimeModels`); `GetAvailable` still excludes them (no auth).
  Full `go test ./... -count=1` verified green.

### council-build-failover-guard — keep build children out of mid-task failover

- **Status:** DONE — shipped on `wave-council-build-failover-guard` as `ebefcbd`,
  merged into `main` as `3b1b73b` (`--no-ff`).
- **Depends on:** —
- **Why:** `wopr-failover` relaxed the subagent failover gate for read-only children
  by exempting `TypeGeneral` writers, but `TypeBuild` also writes code (in its own
  worktree). It is safe today only because `councilBuildHost`'s `Failover` returns
  false unconditionally; if council failover is ever implemented, a build child with
  tool calls would be eligible to rerun over its own edits. Surfaced by the
  `wopr-failover` diff verifier.
- **Files:** `internal/codingagent/subagent/task.go` (gate),
  `coding/session_council_build.go`.
- **Gates:** a test that a `TypeBuild` route with tool calls does not fail over past a
  provider error.
- **Outcome:** added `writesFiles` capability predicate (`TypeGeneral`/`TypeBuild`)
  and routed both the failover gate and the escalation gate through it, so the
  exemption is by capability, not by the council host's current `Failover` return.
  The escalation gate (same file, same loop) was folded in — a provider error sets
  `trigger = "provider error: …"`, so a failover-blocked build child would otherwise
  escalate over its worktree edits. `coding/session_council_build.go` needed no edit.
  Test: `TestBuildWriterDoesNotAdvancePastProviderError` (red pre-fix
  `failovers=1 escalated=1`). No failover eligibility was broadened.

### session-directory — let a session work in a directory other than the process cwd

- **Status:** DONE — shipped on `wave-session-directory` as `9e2c944` + `9062a52` + `b79bd06` + `5a9c2ff`, merged `--no-ff` into `main`. Spec at `docs/plans/2026-10-06-session-directory.md`,
  verified 2026-10-06 by three independent verifier fans (final pass 0 blockers;
  the four residual warnings/nits corrected). The plan is the single source for
  the decisions, files, slices, and gates; this entry records status and
  dependencies only, to avoid the two drifting.
- **Depends on:** —
- **Why:** wopr pins every session to the process cwd. The CLI reads `os.Getwd()`
  with no override (`cmd/wopr/main.go:349`; no directory flag in
  `cmd/wopr/args.go`), and `wopr serve` builds one `host.Host` → one
  `coding.Services` from the startup cwd (`cmd/wopr/serve.go`), so the API's
  `directory` query only *validates* against that single root. opencode lets you
  name the directory to work in; wopr requires `cd` first (CLI) and cannot serve a
  second directory at all (server).
- **Plan:** 3 slices, all launch-time — `--dir` for the CLI and `wopr serve` plus
  the global-layer `server.allowedRoots` guardrail; the `/file` directory
  validation; docs. One root per process; the API `directory` query stays a
  validation against that root (400 on mismatch), not a per-request selector.
  Consequence: `serve-file-list-directory-param` folds into the plan's `/file`
  slice. See the plan for the decisions, files, and per-slice gates.
## Follow-ups surfaced by session-directory

### serve-allowed-roots-fail-open — a malformed global settings.json silently disables the allowlist

- **Status:** DONE — shipped on `wave-serve-allowed-roots-fail-open` as `09bbd65`.
- **Why:** `runServeCommand` reads `server.allowedRoots` from the global layer and
  warns on a settings parse error, then continues. A malformed global
  `settings.json` zeroes the layer, `GetServerAllowedRoots` returns `nil, nil`,
  and `resolveServeRoot` treats an empty list as unrestricted — the guardrail is
  configured but off. Not attacker-triggerable unless the launcher can write
  `~/.wopr/settings.json`, but it is a fail-open robustness gap in the control
  that is supposed to fail closed.
- **Files:** `cmd/wopr/serve.go` (the allowlist read), `internal/codingagent/settings.go`.
- **Gates:** a test that a configured-but-unparsable global settings layer makes
  `wopr serve` refuse to start rather than serve unrestricted.
- **What shipped:** `SettingsManager.GlobalLoadError()` exposes the recorded
  global-layer load error; `globalServeAllowedRoots` in `cmd/wopr/serve.go`
  returns that error instead of the zero layer, and `runServeCommand` exits via
  `failf` before `resolveServeRoot`. An absent or parsed layer with an empty
  allowlist still returns `nil, nil` (unrestricted). Tests:
  `TestGlobalServeAllowedRootsFailsClosedOnMalformedGlobalSettings` and
  `TestRunServeCommandRefusesOnMalformedGlobalSettings`.
- **Known residual:** the guard covers JSON parse errors only. A wrong-typed
  `server`/`allowedRoots` value is swallowed by the shared tolerant decoder
  (`Settings.UnmarshalJSON` → `decodeTolerant`), so it still fails open — split
  out as `serve-allowed-roots-type-mismatch-fail-open` below rather than widened
  into this item.

### serve-allowed-roots-wiring-test — no test covers the global-layer read and the resolveServeRoot call

- **Status:** DONE — shipped on `wave-serve-allowed-roots-wiring-test` as
  `06a850a`; merged into `main`. Added `TestGlobalServeAllowedRootsReadsGlobalLayerOnly`
  (trusted project `settings.json` cannot widen the list) and
  `TestRunServeCommandRefusesRootOutsideGlobalAllowlist` (wiring calls
  `resolveServeRoot` with the global-derived list), both red-proofed against a
  `sm.Get()` read and a dropped call. Test-only; no production change.
- **Why:** `TestResolveServeRoot` exercises the pure helper and
  `TestGetServerAllowedRoots` the getter, but nothing asserts that
  `runServeCommand` reads the allowlist from the global layer (projectTrusted
  false) and actually calls `resolveServeRoot`. A regression reading merged
  project settings, or dropping the call, stays green. The plan calls the
  global-only rule load-bearing.
- **Files:** `cmd/wopr/serve.go`.
- **Gates:** assert a project `settings.json` under the served root cannot widen
  the list, and that `runServeCommand` reads the allowlist from the global layer
  (`projectTrusted` false). The testable helper now exists:
  `globalServeAllowedRoots` (added by `serve-allowed-roots-fail-open`, commit
  `09bbd65`), so the remaining work is the global-only/project-cannot-widen
  assertion at that seam.

### serve-allowed-roots-type-mismatch-fail-open — a wrong-typed server.allowedRoots still fails open

- **Status:** DONE — shipped on `wave-serve-allowed-roots-type-mismatch-fail-open`
  as `c7e9f0b` (+ `3c9904d` case-insensitive bypass fix); merged into `main`.
- **What shipped:** `checkServerRaw` in `internal/codingagent/settings.go` runs in
  `Settings.UnmarshalJSON` before `decodeTolerant` and returns a scoped error when a
  `server` entry present in the raw JSON is not an object, or a `server.allowedRoots`
  entry is present but not an array of strings. Keys are matched with
  `strings.EqualFold` because `encoding/json` resolves struct keys case-insensitively
  (`{"Server":...}`, `{"server":{"AllowedRoots":"x"}}`); an exact-key lookup was a
  bypass. The error propagates through the existing `GlobalLoadError()` plumbing, so
  `wopr serve` refuses to start; `cmd/wopr/serve.go` needed no change.
  `decodeTolerant` and unrelated-key tolerance are untouched. Tests:
  `TestUnmarshalJSONRejectsUndecodableServerGuardrail` (unit, incl. case variants),
  the `wrong-typed-roots` subtest of
  `TestGlobalServeAllowedRootsFailsClosedOnMalformedGlobalSettings`, and
  `TestRunServeCommandRefusesOnUndecodableServerGuardrail`.
- **Why:** `Settings.UnmarshalJSON` routes through `decodeTolerant`
  (`internal/codingagent/settings.go:298-354`), which drops a field whose JSON type
  mismatches its key and retries. So `{"server":5}`,
  `{"server":{"allowedRoots":"x"}}`, `{"server":{"allowedRoots":[5]}}` and
  `{"server":{"allowedRoots":{}}}` all parse successfully: `GlobalLoadError()` is
  `nil`, `Server` is nil or `AllowedRoots` empty, `GetServerAllowedRoots` returns
  `nil, nil`, and `resolveServeRoot` treats the empty list as unrestricted. The
  `serve-allowed-roots-fail-open` guard only catches JSON parse errors, so this
  variant still silently disables the guardrail.
- **Files:** `internal/codingagent/settings.go` (the decoder and the allowlist
  read), `cmd/wopr/serve.go`. Likely needs a "dropped/undecodable key" signal out
  of `decodeTolerant` scoped to `server`, since changing the tolerant decoder's
  repo-wide semantics would affect every setting; design decision required.
- **Gates:** a test that a wrong-typed `server` or `server.allowedRoots` in the
  global layer makes `wopr serve` refuse to start rather than serve unrestricted,
  without changing the tolerant behavior of unrelated settings.

### serve-allowed-roots-unknown-key-fail-open — a misspelled server.allowedRoots key silently disables the allowlist

- **Status:** DONE — shipped on `wave-serve-allowed-roots-unknown-key-fail-open`, merged into
  `main`. Owner decision option (b): a `server` key on the `allow…root` stem that is not the
  exact (case-folded) `allowedRoots`, with no valid `allowedRoots` present, makes `wopr serve`
  refuse to start; any other unrecognized `server` key warns and continues. Surfaced by the
  `serve-allowed-roots-type-mismatch-fail-open` verification (security angle).
- **Why:** the tolerant decoder ignores unknown keys, so `{"server":{"allowedRoot":"/srv"}}`
  (typo) or `{"server":{"allowedRootsX":[...]}}` parses successfully with an empty
  allowlist, `GetServerAllowedRoots` returns `nil, nil`, and `wopr serve` treats the
  empty list as unrestricted — the configured guardrail is silently off. Same
  fail-open class as the wrong-typed variant, but for a key the decoder never
  matches; the type-mismatch fix deliberately did not widen to it.
- **Files:** `internal/codingagent/settings.go` (the allowlist read and/or the
  `server` pre-check).
- **Gates:** a global `settings.json` whose `server` object contains a key that is
  near-miss for `allowedRoots` and no valid `allowedRoots` makes `wopr serve` warn or
  refuse rather than serve unrestricted; unrelated unknown keys stay tolerant.
- **Note:** the design decision is implemented as option (b). `serverKeyIsAllowedRootsNearMiss`
  in `internal/codingagent/settings.go` flags keys on the `allow…root` stem that
  `encoding/json` would not match (including whitespace and invisible `unicode.Cf` padding such as
  a pasted U+200B); `checkServerRaw` fails closed only when a near-miss exists and no valid
  `allowedRoots` was found. Other unrecognized `server` keys ride an unexported
  `Settings.serverUnknownKeys` + `SettingsManager.GlobalUnknownServerKeys`, printed by
  `cmd/wopr/serve.go` as `unrecognized server setting %q ignored`.
- **Known residuals (accepted; spec Open Questions):** JSON `null` for a near-miss or
  `allowedRoots` is treated as absent; an explicit empty `allowedRoots: []` beside a near-miss
  does not fail (empty is the documented unrestricted value); typos with no shared stem
  (`alowedRoots`, `allowedRots`, `roots`, `allowed`) are not caught.

### serve-server-key-near-miss-fail-open — a misspelled outer `server` key still fails open

- **Status:** DONE — shipped on `wave-serve-server-key-near-miss-fail-open`, merged into `main`.
  A new outer-key near-miss predicate (`serverKeyIsServerNearMiss`) catches a top-level key that
  looks like `server` and, when no exact `server` supplied a valid `allowedRoots`, `wopr serve`
  refuses to start naming the key (`"server " is not a known key; did you mean server?`); a valid
  exact `server` wins and the near-miss key warns via the existing unknown-key path. Padded,
  invisible-rune-padded (any Unicode category including interior and non-ASCII case-folding runes),
  and stray-letter-extended outer keys all fail closed; a genuinely unrelated top-level key stays
  tolerant.
- **Known residuals (accepted; spec Open Questions):** name mutations that break the contiguous
  `server` stem (`srvr`, `serve`, `srever`, insertions like `serxver`) and non-ASCII letter
  confusables (`ｓｅｒｖｅｒ`, Cyrillic `ѕerver`) still fail open — catching them needs edit distance,
  which the shipped inner-key item also rejects. A near-miss outer key holding only `port` (no
  `allowedRoots`) also refuses, a deliberate fail-closed extension of the owner's option-(b)
  precedent.
- **Why:** `checkServerRaw` matches the outer key with `strings.EqualFold(key, "server")` and no
  trim, so `{"server ":{"allowedRoots":["/srv"]}}` (or an invisible-rune-padded `server`) is never
  inspected; `encoding/json` also will not match it, the whole `server` object is dropped by
  `decodeTolerant`, `GetServerAllowedRoots` returns `nil, nil`, and `wopr serve` serves
  unrestricted. Same fail-open class as the inner-key item, one level up. Pre-existing, not
  introduced by that item.
- **Files:** `internal/codingagent/settings.go` (the `server` key match in `checkServerRaw`).
- **Gates:** a global `settings.json` whose top-level `server` key is padded/misspelled and holds
  a valid `allowedRoots` makes `wopr serve` refuse rather than serve unrestricted; a genuinely
  unrelated top-level key stays tolerant.

### serve-unknown-server-key-schema-drift — the known-key set duplicates ServerSettings' JSON tags

- **Status:** DONE — shipped on `wave-serve-unknown-server-key-schema-drift`, merged into `main`.
  `checkServerRaw`'s known inner-key set is now derived at runtime from `ServerSettings`' fields via
  the new `isServerSettingKey` helper, which mirrors `encoding/json` field resolution (JSON tag name,
  or the Go field name for an untagged / `json:",opt"` field; unexported and `json:"-"` skipped), so
  a future `ServerSettings` field — tagged or not — can never land in `serverUnknownKeys` or trip the
  `allowedRoots` near-miss refusal. `allowedRoots` array validation and `port`/`hostname` behavior are
  unchanged. No test added (the fix enforces the coupling structurally; spec `docs/plans/2026-10-07-serve-unknown-server-key-schema-drift.md`).
- **Why:** `checkServerRaw` hard-codes the known `server` keys (`port`, `hostname`,
  `allowedRoots`). A future `ServerSettings` field added to the struct but not to that set is
  still applied by `decodeTolerant`, yet lands in the unknown-key list and prints a spurious
  "unrecognized server setting" warning — and, if its name matched the `allow…root` stem, would
  refuse a valid config. A drift hazard, not a current bug.
- **Files:** `internal/codingagent/settings.go`.
- **Gates:** a `ServerSettings` field addition cannot produce a false unknown-key warning
  (derive the known set from the struct tags, or add a test coupling the two).

---

## Follow-ups surfaced by KKCode remote-client testing

### serve-kkcode-v1-compat — KKCode mobile client 404s on connect: missing `/api/health` and sibling routes

- **Status:** READY — surfaced 2026-10-07 testing `wopr serve` against the KKCode iOS client
  (closed-source; UA `KKCode/29694823`). Captured by a logging reverse proxy in front of a real
  opencode 1.18.34 server.
- **Why:** KKCode's first connect call is `GET /api/health`, which wopr does not serve, so the whole
  connect aborts with a 404 before it reaches the V1 surface wopr already implements. A full
  connect → project → session → prompt flow uses exactly these routes: `/api/health`, `/global/health`,
  `/experimental/session`, `/project`, `/global/event` (SSE), `/file`, `POST /session`, `/api/command`,
  `/session/{id}/todo`, `/session/{id}/message`, `/agent`, `/config/providers`, `/config`, `/permission`,
  `/question`, `POST /session/{id}/prompt_async`. wopr already serves all but four routes and one
  status: `GET /api/health` → 404 (opencode answers `{"healthy":true}`); `GET /experimental/session`
  → 404 (same session list as `GET /session`); `GET /config` → 404 (opencode returns an object with
  keys `$schema,agent,command,instructions,mcp,mode,model,permission,plugin,provider,subagent_depth,
  username`); `GET /api/command` → 404 (opencode returns `{location:{directory,project},data:[{name,
  description,template}]}`, e.g. the built-in `init`); `GET /permission` → currently 501, should be
  `200 []`. `/api/status` and `/api/info` are fallback probes that only fire when `/api/health` fails
  (opencode serves them the web-UI HTML), so they are not required.
- **Files:** `internal/codingagent/serve/` (route registration, handlers, DTOs), `docs/site/docs/server.md`
  (endpoint table).
- **Gates:** hermetic tests pinning each new route's status and body shape; a live probe against a real
  listener. Manual: KKCode connects to `wopr serve` through the logging proxy with no 404.
- **Notes:** The V2 tiered-parity plan `docs/plans/2026-10-07-serve-v2-api-parity.md` is NOT needed for
  KKCode (superseded for this client by the capture above); keep it as future work for genuinely V2
  clients. Auth: wopr ignores the client's Basic `Authorization` header and never emits 401, by owner
  decision 2026-10-07.
