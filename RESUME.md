# Resume point

**`serve-prompt-system` — DONE on `wave-serve-prompt-system`, pending merge into `main`. (A concurrent wave, `serve-v1-conformance-and-parity`, is in flight in `.worktrees/wave-serve-v1-conformance`.)**

- **State:** wave branch `wave-serve-prompt-system`, worktree `.worktrees/wave-serve-prompt-system`,
  branched from `main` at `04dc273`. Commits: `01b23b2` (claim + spec), `2ce42a8` (implementation),
  plus the tracker-reconcile commit. Slice gates green: `go build`, `go vet` (touched), `go test -count=1`
  (touched, 462 pass), gate `TestPromptSystemAppliesToTheTurn`, `go tool golangci-lint run` (0 issues),
  `go fix -diff` (empty), `make docs-drift`, `go test -race` (ai + serve).
- **Not run:** `make check` stops at `test-prereqs` because `tmux` is absent, so `make test-integration`
  did not run. The change does not touch the pty/TUI loop, but the integration tier is UNVERIFIED here.
- **Shipped:** per-prompt `system` on both prompt endpoints. opencode parity is **append** (base + `"\n"` +
  text, last), persisted on `UserMessage.System`, echoed on `GET /session/{id}/message` and both SSE user
  frames. Empty string treated as absent (recorded divergence D1 in `BACKLOG.md`).
- **Next step:** merge `wave-serve-prompt-system` `--no-ff` into `main`, prune the worktree. Then the
  sibling S3 items `serve-prompt-no-reply` and `serve-prompt-parts-agent-subtask` remain READY.
- **Do not:** claim the item text's "override" wording — it is an append. The gate asserts the provider
  request, not a model response.
