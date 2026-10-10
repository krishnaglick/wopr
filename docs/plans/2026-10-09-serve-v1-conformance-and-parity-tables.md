# Sidecar — serve-v1-conformance-and-parity tables

Bulk reference for `2026-10-09-serve-v1-conformance-and-parity.md`. Nothing here is a decision; once S1 lands the
generated `testdata/opencode-v1-contract.json` supersedes this file.

**These tables were wrong once.** Rev-1 of the plan claimed 65 endpoints from a hand-written parser that silently
skipped three whole route groups. A verification pass caught it. The counts below come from the corrected extraction;
the generator in S1 exists so no number in this repo is hand-counted again.

## Provenance

| | |
|---|---|
| Repo | `anomalyco/opencode` |
| Tag | `v1.18.34` |
| Commit | `aec0b9a6d8898f68f923aaf08b7306d931fd9d76` |
| Local clone | `~/dev/opencode-src` (shallow, single-branch, detached HEAD) |
| Server the phone talks to | `http://100.109.237.16:4096` → `{"healthy":true,"version":"1.18.34"}` |
| Layout | 1.18.34 is Effect `HttpApi` under `server/routes/instance/httpapi/{groups,handlers}`. The Hono layout (`server/routes/session.ts`) is `dev`-only. **Do not port from `dev`.** |

## Extraction, and the trap in it

Parse `HttpApiEndpoint.<method>("<name>", <Prefix>Paths.<key>)` across `groups/*.ts`, resolving each
`export const <Prefix>Paths` map. The trap: those maps mix three value forms — plain quoted strings
(`file.ts`: `list: "/file"`), template literals (`` session.ts: `status: \`${root}/status\`` ``), and a bare `root`
alias. A parser that only handles template literals reports ~65 endpoints and drops `experimental.ts` (13),
`file.ts` (6), `global.ts` (6), `instance.ts` (12), `mcp.ts` (8) without complaining. **The generator must fail
loudly when its resolved count is below the raw `HttpApiEndpoint(` count**, which is how the error is caught.

## Counts

| | |
|---|---|
| Raw `HttpApiEndpoint(` occurrences | **127** across 19 group files |
| Resolved unique `(method, path)` | **108** |
| wopr routes (`mux.HandleFunc`) | **34** |
| Resolved as mounted by wopr | 23 |
| `phone:false` | 32 |
| **phone-relevant and absent** | **53** |

wopr's 34 routes exceed the 23 that match an upstream endpoint: `/api/health`, `/api/command`, `/agent`,
`/global/event` and friends are wopr or KKCode additions, not gaps. The four `stubbed` rows are real 501s
(`serve.go:74,75,76,85`).

## Endpoint surface — all 108

`phone` = plausibly called by a third-party mobile client. Every `phone:false` row needs a waiver sentence in the
generated contract.

| method | path | name | phone | disposition |
|---|---|---|---|---|
| `GET` | `/agent` | agent | yes | mounted |
| `DELETE` | `/auth/:providerID` | authRemove | yes | absent |
| `PUT` | `/auth/:providerID` | authSet | yes | absent |
| `GET` | `/command` | command | yes | absent |
| `GET` | `/event` | subscribe | yes | absent |
| `GET` | `/experimental/capabilities` | capabilities | yes | absent |
| `GET` | `/experimental/console` | console | yes | absent |
| `GET` | `/experimental/console/orgs` | consoleOrgs | yes | absent |
| `POST` | `/experimental/console/switch` | consoleSwitch | yes | absent |
| `GET` | `/experimental/resource` | resource | yes | absent |
| `GET` | `/experimental/session` | session | yes | mounted |
| `POST` | `/experimental/session/:sessionID/background` | sessionBackground | yes | absent |
| `GET` | `/experimental/tool` | tool | yes | absent |
| `GET` | `/experimental/tool/ids` | toolIDs | yes | absent |
| `GET` | `/experimental/workspace` | list | no | absent |
| `POST` | `/experimental/workspace` | create | no | absent |
| `DELETE` | `/experimental/workspace/:id` | remove | no | absent |
| `GET` | `/experimental/workspace/adapter` | adapters | no | absent |
| `GET` | `/experimental/workspace/status` | status | no | absent |
| `POST` | `/experimental/workspace/sync-list` | syncList | no | absent |
| `POST` | `/experimental/workspace/warp` | warp | no | absent |
| `DELETE` | `/experimental/worktree` | worktreeRemove | yes | absent |
| `GET` | `/experimental/worktree` | worktree | yes | absent |
| `POST` | `/experimental/worktree` | worktreeCreate | yes | absent |
| `POST` | `/experimental/worktree/reset` | worktreeReset | yes | absent |
| `GET` | `/file` | list | yes | mounted |
| `GET` | `/file/content` | content | yes | mounted |
| `GET` | `/file/status` | status | yes | mounted |
| `GET` | `/find` | findText | yes | absent |
| `GET` | `/find/file` | findFile | yes | mounted |
| `GET` | `/find/symbol` | findSymbol | yes | absent |
| `GET` | `/formatter` | formatter | yes | absent |
| `GET` | `/global/config` | configGet | yes | absent |
| `PATCH` | `/global/config` | configUpdate | yes | absent |
| `POST` | `/global/dispose` | dispose | yes | absent |
| `GET` | `/global/event` | event | yes | mounted |
| `GET` | `/global/health` | health | yes | mounted |
| `POST` | `/global/upgrade` | upgrade | yes | absent |
| `POST` | `/instance/dispose` | dispose | yes | absent |
| `POST` | `/log` | log | yes | absent |
| `GET` | `/lsp` | lsp | yes | absent |
| `GET` | `/mcp` | status | yes | absent |
| `POST` | `/mcp` | add | yes | absent |
| `DELETE` | `/mcp/:name/auth` | authRemove | yes | absent |
| `POST` | `/mcp/:name/auth` | authStart | yes | absent |
| `POST` | `/mcp/:name/auth/authenticate` | authAuthenticate | yes | absent |
| `POST` | `/mcp/:name/auth/callback` | authCallback | yes | absent |
| `POST` | `/mcp/:name/connect` | connect | yes | absent |
| `POST` | `/mcp/:name/disconnect` | disconnect | yes | absent |
| `GET` | `/path` | path | yes | absent |
| `GET` | `/pty` | list | no | absent |
| `POST` | `/pty` | create | no | absent |
| `DELETE` | `/pty/:ptyID` | remove | no | absent |
| `GET` | `/pty/:ptyID` | get | no | absent |
| `PUT` | `/pty/:ptyID` | update | no | absent |
| `GET` | `/pty/:ptyID/connect` | connect | no | absent |
| `POST` | `/pty/:ptyID/connect-token` | connectToken | no | absent |
| `GET` | `/pty/shells` | shells | no | absent |
| `GET` | `/session` | list | yes | mounted |
| `POST` | `/session` | create | yes | mounted |
| `DELETE` | `/session/:sessionID` | remove | yes | mounted |
| `GET` | `/session/:sessionID` | get | yes | mounted |
| `PATCH` | `/session/:sessionID` | update | yes | mounted |
| `POST` | `/session/:sessionID/abort` | abort | yes | mounted |
| `GET` | `/session/:sessionID/children` | children | yes | absent |
| `POST` | `/session/:sessionID/command` | command | yes | absent |
| `GET` | `/session/:sessionID/diff` | diff | yes | mounted |
| `POST` | `/session/:sessionID/fork` | fork | yes | mounted |
| `POST` | `/session/:sessionID/init` | init | yes | absent |
| `GET` | `/session/:sessionID/message` | messages | yes | mounted |
| `POST` | `/session/:sessionID/message` | prompt | yes | mounted |
| `DELETE` | `/session/:sessionID/message/:messageID` | deleteMessage | yes | absent |
| `GET` | `/session/:sessionID/message/:messageID` | message | yes | absent |
| `DELETE` | `/session/:sessionID/message/:messageID/part/:partID` | deletePart | yes | absent |
| `PATCH` | `/session/:sessionID/message/:messageID/part/:partID` | updatePart | yes | absent |
| `POST` | `/session/:sessionID/permissions/:permissionID` | permissionRespond | yes | mounted |
| `POST` | `/session/:sessionID/prompt_async` | promptAsync | yes | mounted |
| `POST` | `/session/:sessionID/revert` | revert | yes | mounted |
| `DELETE` | `/session/:sessionID/share` | unshare | yes | absent |
| `POST` | `/session/:sessionID/share` | share | yes | absent |
| `POST` | `/session/:sessionID/shell` | shell | yes | absent |
| `POST` | `/session/:sessionID/summarize` | summarize | yes | absent |
| `GET` | `/session/:sessionID/todo` | todo | yes | mounted |
| `POST` | `/session/:sessionID/unrevert` | unrevert | yes | absent |
| `GET` | `/session/status` | status | yes | mounted |
| `GET` | `/skill` | skill | yes | absent |
| `POST` | `/sync/history` | history | no | absent |
| `POST` | `/sync/replay` | replay | no | absent |
| `POST` | `/sync/start` | start | no | absent |
| `POST` | `/sync/steal` | steal | no | absent |
| `POST` | `/tui/append-prompt` | appendPrompt | no | absent |
| `POST` | `/tui/clear-prompt` | clearPrompt | no | absent |
| `GET` | `/tui/control/next` | controlNext | no | absent |
| `POST` | `/tui/control/response` | controlResponse | no | absent |
| `POST` | `/tui/execute-command` | executeCommand | no | absent |
| `POST` | `/tui/open-help` | openHelp | no | absent |
| `POST` | `/tui/open-models` | openModels | no | absent |
| `POST` | `/tui/open-sessions` | openSessions | no | absent |
| `POST` | `/tui/open-themes` | openThemes | no | absent |
| `POST` | `/tui/publish` | publish | no | absent |
| `POST` | `/tui/select-session` | selectSession | no | absent |
| `POST` | `/tui/show-toast` | showToast | no | absent |
| `POST` | `/tui/submit-prompt` | submitPrompt | no | absent |
| `GET` | `/vcs` | vcs | yes | absent |
| `POST` | `/vcs/apply` | vcsApply | yes | absent |
| `GET` | `/vcs/diff` | vcsDiff | yes | absent |
| `GET` | `/vcs/diff/raw` | vcsDiffRaw | yes | absent |
| `GET` | `/vcs/status` | vcsStatus | yes | absent |

## `PromptInput` — the request contract for both prompt endpoints

`packages/opencode/src/session/prompt.ts:1499`. `groups/session.ts:70` derives the wire payload as
`Schema.omit(SessionPrompt.PromptInput.fields, ["sessionID"])`, so this table *is* the contract for
`POST /session/:id/prompt_async` and `POST /session/:id/message`. Verified field-by-field against the source.

| field | wopr | slice | gate that would prove it |
|---|---|---|---|
| `messageID` | honored | done | `TestPromptMessageIDHonoredEndToEnd` |
| `model` | honored | done 2026-10-10 | `TestPromptAsyncAppliesTheClientModel` |
| `agent` | dropped | S2 | behavioral: persisted user message reports the agent |
| `system` | dropped | S3 | behavioral: override reaches the turn |
| `noReply` | dropped | S3 | behavioral: user message persists, no assistant turn |
| `parts: agent\|subtask` | dropped | S3 | behavioral |
| `variant` | dropped | S4 | behavioral; **blocked** on the shapes sidecar |
| `tools` | dropped | S5 | honor or waive — open question 2 |
| `format` | dropped | S5 | behavioral: structured output tool fires |

Upstream resolves the model in `createUserMessage` (`prompt.ts:646`), not at the HTTP boundary:

```ts
const model = input.model ?? ag.model ?? (yield* currentModel(input.sessionID))
```

which is why wopr's swap runs in the run-slot `before` hook rather than at accept time. Deliberate, not incidental.

## Why the field table gets no reflection-based test

A test that reflects over `promptBody`'s JSON tags and diffs it against this list is green for `model` — the tag was
present and the value was ignored. It is the same blind spot as a route table. The structural trick that makes
`isServerSettingKey` (`internal/codingagent/settings.go:395`) un-silenceable does **not** transfer: there both
sides are Go values in one package, so they cannot drift. Here one side is a TypeScript schema with no in-repo
representation, and the other is a generated JSON file. The un-silenceable oracle for a field is a test that asserts
what the field *did*.

## KKCode traffic observed so far

`/tmp/wopr-proxy.log`, 81 requests, UA `KKCode/29694823`. Ephemeral — copy it somewhere durable if it is to stay
evidence. The prompt bodies are the only ground truth about the client:

```json
{"messageID":"msg_optimistic_muzwmw43_1","parts":[{"type":"text","text":"Hi"}],
 "agent":"explore","model":{"providerID":"opencode-go","modelID":"deepseek-v4.1-flash"}}
```

Every captured prompt carries `agent` and `model`. `model` is honored; `agent` is the live gap (S2).
