# Resume point

**`serve-limit-history-tokens` — DONE on `wave-serve-limit-history-tokens`, pending merge into `main`. (A concurrent wave, `serve-v1-conformance-and-parity`, is also in flight in `.worktrees/wave-serve-v1-conformance`.)**

- **State:** wave branch `wave-serve-limit-history-tokens`, worktree `.worktrees/wave-serve-limit-history-tokens`,
  branched from `main` at `8432866`. Commits: `e10a076` (claim), `8675501` (spec), `9655b10` (implementation),
  plus the tracker-reconcile commit. Slice gates green: `go build`, `go vet`, `go test -count=1 ./...`
  (58 packages), golangci-lint, `go fix -diff` (empty), docs-drift, `make test-race`.
- **Not run:** `make check` stops at `test-prereqs` because `tmux` is absent, so `make test-integration` did
  not run. The change does not touch the pty/TUI loop, but the integration tier is genuinely UNVERIFIED here.
- **Shipped:** `coding.ServicesOptions.MaxHistoryTokens *int` (`nil` → 80% of the model's effective window,
  `0` → disabled, `N` → cap) plumbed from `wopr serve --max-history-tokens`; new `compaction.TrimMessages`
  caps the per-request context at the end of `prepareRequest` (`coding/session_transcript.go:142`). Gate
  `TestMaxHistoryTokensTrimsOldest` (`coding`) fails if the trim is removed.
- **Next step:** merge `wave-serve-limit-history-tokens` `--no-ff` into `main`, prune the worktree, then pick
  up `serve-transcript-ram-cap` (new READY follow-up) — the item's RAM premise is not solved here.
- **Do not:** claim this capped retained RAM. It bounds the per-request context only; `inner.entries` remains
  append-only. Also pending: owner ratification of the global 80% default (applies to TUI/print/host too).
