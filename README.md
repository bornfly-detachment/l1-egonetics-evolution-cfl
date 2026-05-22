# egonetics-evolution

Egonetics Evolution Layer — autonomous LLM training experiments.

## Philosophy

karpathy autoresearch style: single-file, agent-editable, fixed time budget, metric-driven keep/discard.

Three files:

| File | Edited by | Purpose |
|------|-----------|---------|
| `prepare.py` | Human (fixed) | Training data generation via SEAI |
| `train.py` | Agent | Model architecture + training loop (GPT + MoE) |
| `mini_dsv4.py` | Agent | Mini DeepSeek-V4 architecture (MLA + MoE) |

## Dependency

Calls SEAI for data preparation:
```
/Users/Shared/SubjectiveEgoneticsAI/
```

## Python 环境（两个 venv，不要混）

| 脚本 | 环境 | 命令 |
|------|------|------|
| `train.py` / `mini_dsv4.py` | 本仓 `.venv`（Python 3.11 + torch） | `source .venv/bin/activate` 后用 `python …` |
| `prepare.py` | `~/llama-factory/venv`（SEAI 同款） | `./scripts/run-prepare.sh --exp-id <id>` |

首次建训练环境：

```bash
cd /Users/Shared/egonetics-evolution
python3.11 -m venv .venv && .venv/bin/pip install "torch>=2.5"
```

Cursor 解释器选 **`egonetics-evolution/.venv`**。详见 `project_config.md`。

## Scale Presets (mini_dsv4.py)

```bash
source .venv/bin/activate
python mini_dsv4.py --scale tiny    # ~25M  params, quick test
python mini_dsv4.py --scale small   # ~150M params, trainable locally
python mini_dsv4.py --scale medium  # ~500M params
python mini_dsv4.py --scale base    # ~850M params, vs Qwen2.5-0.8B
```

## Experiment Runner

Driven by TypeScript `experiment-runner.ts` via `seai-bridge.ts`:
1. `./scripts/run-prepare.sh --exp-id <id>` — generates training data (SEAI)
2. `train.py --exp-id <id>` or `mini_dsv4.py --exp-id <id>` — runs training (`.venv`)
3. Parses val_loss from stdout JSON
4. Keeps or discards based on metric improvement

## Ecosystem runtime CFL (2026-05-18)

The PRD-driven Evolution ecosystem runtime lives in `ecosystem-runtime/`. It is a Go runtime that reuses the standalone P/R/V/S CFL projects and preserves the corrected 2026-05-17/18 PRD semantics: open tasks, non-exclusive claims, independent seed runtimes, free communication, V-owned evaluation dimensions, and user resource injection rather than user payment.

See `CHRONICLE.md` and `ecosystem-runtime/README.md` for the Chronicle ↔ Git handoff record.
