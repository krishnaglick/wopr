# Resume point

**`serve-v1-conformance-and-parity` — spec re-derived under the owner's authority rule. No code written yet. Wave branch carries two doc commits.**

- **State:** wave `wave-serve-v1-conformance`, worktree `.worktrees/wave-serve-v1-conformance`, branched from `main`
  at `92d3093`. Branch commits: `17fddc9` (resume point), `a7a671c` (authority rule + round-3 corrections). **Zero
  code commits.** `main` carries `9c52e59`, `b5349b9`, `92d3093` from this session.
- **Owner directive, now in `AGENTS.md`:** opencode `v1.18.34` (commit `aec0b9a`) is the specification for `wopr serve`.
  Where wopr's behavior, a DTO, a route, or any `docs/` file disagrees, opencode wins and the document is wrong —
  including `docs/site/docs/server.md` and the captured KKCode shapes sidecar. Divergences survive only as recorded
  product decisions; the ratified ones are named in the rule so it cannot silently overturn them.
- **Verification history:** fan round 1 → BLOCK (route count wrong by half; central test could not fail). Round 2 →
  BLOCK again (banned test categories; `agent` gate named an observable that did not exist; no named mutations).
  Round 3 → BLOCK on four *editorial* items only. Those four were fixed and self-checked by reading the files;
  **a round 4 fan was deliberately not run.** If the next session wants a clean chain, fan the revision before coding.
- **Batch narrowed to S1 + S2.** S3 was dropped from the wave: `system` and `parts: agent|subtask` have no identified
  observable, and a slice whose gate cannot be written should not be dispatched. S3–S6 are filed READY for `ocean`.
- **Next step: S1.** `automation/gen-opencode-contract.py` (with the loud under-count guard), the generated
  `internal/codingagent/serve/testdata/opencode-v1-contract.json`, and the `routeMux` addition to `serve.go`. Then
  file every deferred slice to `BACKLOG.md` as READY. Then S2 (`agent`, 400 on unknown, gate
  `TestPromptAsyncAppliesTheClientAgent` with its named mutation).
- **Do not:** quote an endpoint count in prose unless the generator reproduces it — a hand parser under-counted twice
  (65, then 108; the truth is 127) and both errors shipped into a committed document before review caught them.
