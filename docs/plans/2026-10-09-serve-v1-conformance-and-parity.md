# serve-v1-conformance-and-parity — stop discovering KKCode gaps one bug at a time

## Orchestrator brief

- **Files touched (S1 only):**
  - `automation/gen-opencode-contract.py` — **new**. Reads a pinned opencode checkout, emits the contract JSON.
    Reproducible, so no number in this repo is hand-counted.
  - `internal/codingagent/serve/testdata/opencode-v1-contract.json` — **new**, generated, committed.
  - `internal/codingagent/serve/serve_conformance_test.go` — **new**. Two tests, both behavioral.
  - `internal/codingagent/serve/serve.go` — **one** addition: a `routeMux` type embedding `*http.ServeMux`. The 34
    `mux.HandleFunc` call sites stay byte-identical.
  - `BACKLOG.md` — one item per slice below.
  - `docs/plans/2026-10-09-serve-v1-conformance-and-parity-tables.md` — sidecar.
- **Gates (every slice):**
  - `go build ./...`
  - `go vet ./...`
  - `go test -count=1 ./internal/codingagent/serve ./internal/codingagent`
  - `go tool golangci-lint run ./internal/codingagent/...`
  - `go fix -diff ./...` empty
  - before merge: `bash -lc 'make check'`
- **Done when:** every endpoint wopr does not serve is classified `not-phone` with a reason, or is an open backlog
  item — and every `PromptInput` field is either honored with a behavioral test proving it, or waived in writing.
- **Out of scope:** opencode **V2** (`/api/*`); `/tui/*`, `/pty/*`, `/sync/*`, `/experimental/workspace/*`,
  `/experimental/control-plane/*`; auth/TLS/CORS; any change to `coding/`, `agent/`, `ai/`, `tui/`.
- **Open questions:** two, §6. S1 blocks on neither.

---

## 1. What a static harness can and cannot catch

This is the whole design, and it is narrower than a first pass suggests.

**A route ledger catches missing routes.** KKCode calls `POST /session/:id/share`, wopr 404s, someone notices. A
committed table turns that into a red test the moment upstream adds the endpoint.

**A static harness cannot catch a dropped field.** The bug that started this was `prompt_async` decoding `body.Model`
and never using it — the route was mounted, the field was *present in the struct*, and the handler ignored it. A
reflection-over-JSON-tags diff is **green** for exactly that. So would a route table, a shape snapshot, or a golden
replay of captured traffic: none of them observe that a value was parsed and then not used.

Therefore:

| gap class | caught by |
|---|---|
| route missing entirely | the ledger (S1) |
| route added upstream, not reviewed | the ledger (S1) |
| **field present but not honored** | **only a behavioral test per field** — the job of S2+ |
| DTO shape drift | `assertInfoShape`-style tests that already exist |

Anything that claims to close the third row without a behavioral test is decoration. S1 does not, and this plan does
not pretend to.

## 2. Slices

Each is one feature, gates green, independently committable. A waiver-only slice is not a slice — it is a snapshot
edit folded into the slice that would have implemented it.

### S1 — the route ledger

- `automation/gen-opencode-contract.py`: parse `HttpApiEndpoint.<method>("name", <Prefix>Paths.<key>)` across
  `packages/opencode/src/server/routes/instance/httpapi/groups/*.ts`, resolve each `<Prefix>Paths` map (which mixes
  plain strings, template literals and a `root` alias — a naive parse drops whole groups; this bit me once already),
  and emit `{pinned, endpoints:[{method,path,name,group,phone,disposition,waiver}]}`. `disposition` is
  `mounted` | `stubbed` | `absent`. `phone:false` rows must carry a `waiver` sentence.
- `serve.go`: `type routeMux struct { *http.ServeMux; routes []route }` with its own `HandleFunc` that records.
  Every existing call site is unchanged text; the `ServeMux` panic on a bad pattern still fires because the embedded
  method is still what registers.
- `serve_conformance_test.go`, **two** tests:
  - `TestPhoneEndpointsAreMountedOrAccountedFor` — every `phone:true` endpoint is `mounted`, or is `stubbed`/`absent`
    **with a non-empty waiver**. The four current 501s (`fork`, `revert`, `diff`, `permissions`) are recorded as
    `stubbed` with a waiver, so a 501 on a phone route is a tracked gap rather than a silent one.
  - `TestServedRoutesAreClassified` — every route in `routeMux.routes` appears in the contract with a disposition.
    Fails when wopr adds a route nobody reviewed.

Both are behavioral (a route is mounted or it is not), so neither is the banned snapshot/change-detector category.
The contract JSON is generated, not hand-maintained, so editing it to silence a failure is not a one-string change —
it is a generator + review. **No network at test time**; regeneration is a manual command recorded in the file header.

### S2 — `agent` on both prompt endpoints

`body.Agent` resolved against the names `GET /agent` already serves (`serve_handlers.go:532`), applied in the same
run-slot `before` hook the model now uses. **Gate: a behavioral test** — a prompt naming a known agent runs and the
persisted user message reports that agent; an unknown name is `400 {"error":"agent not found: …"}`. Per §1 the test
must assert the *effect*, not that the field parses. Every captured KKCode prompt carries `agent`, so this is the
live gap.

### S3 — `system`, `noReply`, `parts: agent|subtask`

Grouped because each is small and each ships its own behavioral assertion: the persisted user message / turn
reflects the override. `noReply` needs the host to persist without a turn — check `SendContentWithPreflight` first;
if that is not possible, the slice becomes a waiver, not a stub.

### S4 — `variant`

Separate because it is a trap, not because it is big. wopr's `sessionModel.Variant` is a hardcoded `"default"` and
the shapes sidecar records that as *captured* KKCode behavior; `providerModelDTO.Variants` is an empty map. Changing
it alters a DTO the phone renders from. Requires first populating `Variants` from the model DB, then a behavioral
test on the emitted `variant`. Do not start without re-reading
`docs/plans/2026-10-07-serve-kkcode-dto-shape-parity-shapes.md:102,125`.

### S5 — `tools`, `format`

`tools`: upstream deprecates it in favour of session permissions, and wopr has a permissions model. Likely a
permanent waiver; decide, write it down, done. `format: json_schema`: needs a structured-output tool; wopr's own
system prompt already promises the capability. Largest single item — may be several slices.

### S6 — the absent phone endpoints

From the generated contract: `/session/:id/share` + `unshare`, `summarize`, `message/:messageID` get/delete,
`children`, part edit/delete, `unrevert`, `shell`, `command`, `init`, `PATCH /config`. Grouped into ~4 slices by
shared prerequisite, not one-per-endpoint. `shell`/`command`/`init` are one investigation first: ask whether the
host can do them at all, and emit waivers for whatever it cannot. A 501 that becomes a permanent answer is a waiver,
not a route.

## 3. What this does not do

- It does not implement opencode. The `phone:false` majority stays unimplemented **on the record**, which is the
  actual win: a decision instead of an omission.
- It does not test KKCode. Its call set beyond the 81 captured requests in `/tmp/wopr-proxy.log` is unknown and
  unknowable — it is closed source. The ledger bounds the server contract, which is the part wopr owns.
- It does not add a network dependency. `make check` stays hermetic.

## 4. Risks

- **The contract rots.** opencode moves past `v1.18.34`. Mitigated by recording the commit in the generated file and
  a regeneration command in its header; failure mode is a stale file, not a wrong test. Staleness is *not* auto-detected
  — deliberately, since a network check would break hermeticity. Revisit on the next opencode upgrade.
- **S1 touches `serve.go`'s mux construction.** Every serve test must pass unedited.
- **Upstream layout churn.** v1.18.34 is Effect `HttpApi`; earlier tags were Hono. The generator must fail loudly on
  an unrecognized layout rather than silently under-count — that failure mode already produced one wrong number.

## 5. Adjacent: the parked V2 plan is rotting

`docs/plans/2026-10-07-serve-v2-api-parity.md` is parked (BACKLOG:1291: "NOT needed for KKCode … future work for
genuinely V2 clients"). Its model of record is `/tmp/openapi-v2.json` — **ephemeral** — and it has no harness, so
nothing would notice the loss. One backlog line, not this plan's work. Decide: check in, re-download with a recorded
digest, or accept the loss in writing.

## 6. Open questions

1. **Unknown `agent` → 400 or fall back?** Upstream falls back to the default agent. wopr answering 400 is a louder,
   safer divergence for a client that typos. S2 blocks on an owner decision.
2. **`tools` — honor or permanently waive?** §S5. A waiver shrinks the work; a decision is needed either way.
