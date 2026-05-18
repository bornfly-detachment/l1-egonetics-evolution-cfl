# PRD coverage: 2026-05-17/18 Evolution

| PRD section | Implementation |
| --- | --- |
| §3 四主要矛盾 | 生态状态、初始 3 种子、公开 Task、公平 audit、资源死亡机制。 |
| §4.1 AI 种子分配 | `register_direction` 为每个方向创建 `extreme_a` / `median` / `extreme_b`。 |
| §4.2 Task 体系 | `TaskInput` / `Task` 覆盖目标、资源、宪法、L0/L1/L2、公开可见。 |
| §4.2.1 Task 持续开放 | `Task.Status` 保持 `open`；`submit_solution` 允许任何活种子重复提交；`SolutionIDs` 保留全历史解。 |
| §4.3 资源体系 | 覆盖 API Token、bornfly 代理、网络、公开知识；用户/研究员注入新资源走 `resource_injection`，不是商业支付模型。 |
| §4.4.1 奖励两档 | `base_reward` / `breakthrough_reward` 分离；首次达标给 base，V 判定突破给 breakthrough。 |
| §4.4.1 V 独立评价维度 | `submit_solution` 真实执行 V CFL；运行时不写死维度、权重、聚合算法，只保存 `dimension_vector`/raw。 |
| §4.5 公平与路径依赖 | 所有 Task 公开广播；同方向 3 种子；多解并存，新解可上位。 |
| §4.6 范式种子 | `DirectionInput.paradigm_seed_text` 是范式种子入口。 |
| §4.7 任务市场 | 当前实现研究员 Task 入口与 `resource_injection` 资源注入；reward 从全局资源池结算，外部用户产品化仍为 future/open option。 |
| §4.8 独立 runtime + 通信 | `SeedRuntime` + `runtime_tick` + `send_message` 实现 goroutine 级 runtime 记录和 R message bus。 |
| §5 L0/L1→L2 | `advance_stage` 切换阶段；L2 消耗 `api_token` 会取消资格并终止 runtime。 |
| §6 boot + escalation | `bootstrap` 和 `escalate` / Inbox 实现 bornfly boot + escalation handler 边界。 |
| §7 硬约束 | `audit` 覆盖内部 CFL、bornfly 非 runtime operator、有限资源、独立 runtime、L2 API 禁用。 |
| §12 CFL 对接 | 默认 registry 复用 `p-cfl`、`value`、`r-cfl`、`s-cfl`。 |
| §13 波 1-3 重定义 | 当前实现覆盖独立 runtime/message bus、open problem、多解并存、V 真实 exec、两档奖励。 |

## Known gaps

- V CFL 当前沿用已有 `value/` 二进制；“按任务类型独立建立评价维度”的专用 V 接口仍待 V-CFL PRD 后续增强。
- 研究员手动 Task 难度偏倚、范式种子拆解、失败 Task 处理、长期自治推进仍属 PRD §15 open question。
