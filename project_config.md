# Egonetics 工作区 — 项目配置（稳定信息）

> Agent：每轮对话开始读取本文件 + `workflow_state.md`。勿把临时任务写进本文件。

## 仓库路径与职责

| 仓库 | GitHub（`main`） | 本地路径（仓库根 = 目录根） | 职责 |
|------|------------------|---------------------------|------|
| **egonetics-evolution** | `bornfly-detachment/egonetics-evolution` | `/Users/Shared/egonetics-evolution` | LLM 训练；`ecosystem-runtime/` Go |
| **egonetics**（Web） | `bornfly-detachment/egonetics` | `/Users/Shared/egonetics` | React/Vite + Express/SQLite |
| **egonetics-desktop** | `bornfly-detachment/egonetics-desktop` | `/Users/Shared/egonetics-desktop` | Electron 桌面壳 |

> **注意**：旧版 `/Users/Shared/egonetics`（内含 `main/` 子目录、非 git 根）为历史遗留，**已删除**。现行路径指从 GitHub **重新 clone** 后、仓库根即该文件夹根目录的树。

### 已归档 / 勿用

| 名称 | 说明 |
|------|------|
| **`prvse-world-workspace`** | GitHub 已 `archive`（2026-05-18）。原 PRVSE 共享 kernel/chronicle/L0–L2；**后继：`egonetics-core`**。勿 clone、勿软链 `.env`、勿把 `/Users/Shared/prvse_world_workspace` 当权威 |
| 旧 `/Users/Shared/egonetics` 布局 | 含 `main/` 子目录的嵌套目录 — 已删除 |
| `/Users/Shared/egonetics-opencode` | OpenCode worktree，勿新功能 |
| `/Users/Shared/opencode-egonetics-desktop` | 旧 desktop 目录名 |

> `egonetics` 仓内 `SETUP.md` / `setup.sh` / `server/*` 若仍写 `prvse_world_workspace`，属**文档与代码滞后**，以本表为准。

## 前端：clone（目录不存在或为空时）

```bash
git clone git@github.com:bornfly-detachment/egonetics.git /Users/Shared/egonetics
cd /Users/Shared/egonetics && git checkout main && git pull origin main

git clone git@github.com:bornfly-detachment/egonetics-desktop.git /Users/Shared/egonetics-desktop
cd /Users/Shared/egonetics-desktop && git checkout main && git pull origin main
```

日常：`git pull origin main`。

**环境变量**：在 `egonetics` 根目录与 `server/` 各建 `.env`（不进 git）；**不要**软链到 `prvse_world_workspace/config/`。Kernel/契约走 `@egonetics/core`（`../egonetics-core`）。`egonetics/SETUP.md` 中 prvse 相关步骤已过时。

## 工作区

- **打开**：`/Users/Shared/egonetics-dev.code-workspace`（evolution + egonetics + egonetics-desktop）
- **规则锚点**：`egonetics-evolution/.cursor/rules/`

## 外部依赖（只读默认）

| 名称 | 路径 |
|------|------|
| **SEAI** | `/Users/Shared/SubjectiveEgoneticsAI` |
| **实验状态** | `~/.egonetics/experiments/<exp-id>/` |
| **egonetics-core** | `/Users/Shared/egonetics-core` | 接替已归档的 `prvse-world-workspace`；compiler/runtime/contract |

## PRVSE 边界

- 前端两仓 = UI / 协作；**SEAI** = 训练与编排内核
- 可配置结构须 **CRUD 端到端**

## Python 环境（双 venv，按脚本切换）

| 脚本 | 解释器 | 怎么跑 |
|------|--------|--------|
| `train.py` / `mini_dsv4.py` | `.venv/bin/python` | `source .venv/bin/activate` → `python …` |
| `prepare.py` | `~/llama-factory/venv/bin/python` | `./scripts/run-prepare.sh --exp-id <id> [--data-type sft\|grpo]` |

- **Cursor / Pylance / Ruff**：固定选 **`egonetics-evolution/.venv`**（`.vscode/settings.json`）。
- **不要** 用 conda `base` 或把 llama-factory venv 设为 Cursor 默认解释器。
- 自定义 llama-factory 路径：`LLAMA_FACTORY_VENV=/path/to/venv ./scripts/run-prepare.sh …`

首次建训练 venv：

```bash
cd /Users/Shared/egonetics-evolution
python3.11 -m venv .venv && .venv/bin/pip install "torch>=2.5"
```

## 验证命令

### Evolution

```bash
cd /Users/Shared/egonetics-evolution
source .venv/bin/activate
python mini_dsv4.py --scale tiny
./scripts/run-prepare.sh --exp-id <id>
python train.py --exp-id <id>
cd ecosystem-runtime && go test ./...
```

### Web / Desktop

```bash
cd /Users/Shared/egonetics && npm run lint && npm run build
cd /Users/Shared/egonetics-desktop && npm run lint && npm run build && npm run dev
```
