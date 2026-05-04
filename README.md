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

## Scale Presets (mini_dsv4.py)

```
python mini_dsv4.py --scale tiny    # ~25M  params, quick test
python mini_dsv4.py --scale small   # ~150M params, trainable locally
python mini_dsv4.py --scale medium  # ~500M params
python mini_dsv4.py --scale base    # ~850M params, vs Qwen2.5-0.8B
```

## Experiment Runner

Driven by TypeScript `experiment-runner.ts` via `seai-bridge.ts`:
1. `prepare.py --exp-id <id>` — generates training data
2. `train.py --exp-id <id>` or `mini_dsv4.py --exp-id <id>` — runs training
3. Parses val_loss from stdout JSON
4. Keeps or discards based on metric improvement
