# Workflow State

> 见 `project_config.md`。

## 当前

- **active_task**: Python 双环境已落地（`.venv` 训练 + `run-prepare.sh` → llama-factory）
- **workspace_focus**: evolution + egonetics + egonetics-desktop
- **last_updated**: 2026-05-20

## 最近决策

- 训练：`train.py` / `mini_dsv4.py` → 本仓 `.venv`（3.11 + torch）
- 数据：`prepare.py` → `~/llama-factory/venv` via `scripts/run-prepare.sh`（已验证 SEAI import）
- Cursor 解释器仅绑 `.venv`；debug 配置含 prepare（llama-factory python）

## 待办 / 阻塞

- （无）

## 备注

- `llama-factory` venv 路径可通过 `LLAMA_FACTORY_VENV` 覆盖
