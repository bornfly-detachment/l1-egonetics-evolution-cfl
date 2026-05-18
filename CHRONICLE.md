# egonetics-evolution Chronicle ↔ Git ledger

- Recorded at: 2026-05-18 (Asia/Shanghai)
- GitHub repository: https://github.com/bornfly-detachment/egonetics-evolution
- Local implementation source: /Users/Shared/codex-workspace/evolution
- Local Git sync clone: /Users/Shared/egonetics-evolution
- PRD source: /Users/Shared/product-docs/prd-core/2026-05-17-PRD-evolution.md
- Runtime scope: Evolution ecosystem Go runtime under `ecosystem-runtime/`, reusing P/R/V/S CFLs rather than replacing them.
- Repository coexistence rule: the pre-existing Python training code remains in the repository root; the PRD ecosystem runtime is isolated in `ecosystem-runtime/` to avoid overwriting an existing Claude Code-created project.
- Verification command: `cd ecosystem-runtime && go test ./...`

## 2026-05-18 handoff record

This record anchors the Evolution ecosystem runtime to Git without destroying the earlier egonetics-evolution training repository history. Future PRD-runtime work should modify `ecosystem-runtime/` unless an explicit repository split is approved.
