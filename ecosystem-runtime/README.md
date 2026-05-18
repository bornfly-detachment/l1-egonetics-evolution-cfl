# Evolution ecosystem runtime

Evolution 是基于 `2026-05-17-PRD-evolution.md` 最新 5-18 范式纠错后的最小可运行生态系统实现。它不是新的 PRVSE 第五层，也不是新 CFL；它复用仓库里已经实现的 P/R/V/S CFL，把它们组合成一个封闭、稀缺资源驱动的多 AI 种子生存竞争运行时。

## 已实现范围（波 1-3 MVP，5-18 修正版）

- 生态启动：`bootstrap` 创建封闭生态、PRVSE/CFL 注册表、有限资源池、自治 chronicle。
- 独立种子 runtime：每个种子有 `runtime:*` 记录、goroutine 级 work-loop tick、死亡后 runtime 终止。
- 研究方向：每个方向自动分配 3 个 AI 种子：`extreme_a`、`median`、`extreme_b`。
- Open Task：Task 是持续开放问题，永不关闭；取消 claim 排他；所有历史解进入 chronicle / `solutions`。
- 多解并存：Task 维护 `best_solution_ids` / `best_solution_ref`，新解上位但旧解永存。
- 奖励两档：首次达标给 `base_reward`；已完成 Task 上由 V 判定突破时给 `breakthrough_reward`。
- V 真实执行：`submit_solution` 会执行注册的 V CFL，不接受外部直接传入 verdict；V 输出维度向量和突破判定，运行时只透传。
- R message bus：`send_message` / `runtime_tick` 记录种子间自由通信和 Task 广播。
- 资源体系：`api_token`、`bornfly_proxy`、`network_lookup`、`public_knowledge`；用户/研究员可通过 `resource_injection` 注入新资源，语义不是商业支付。
- L2 守门：种子进入 L2 后 `external_api_allowed=false`；L2 消耗 `api_token` 会取消资格并终止 runtime。
- escalation：异常进入 Inbox，bornfly 只作为 boot loader + escalation handler，不是 runtime operator。
- audit：检查内部 CFL 注册、3 种子规则、公开开放 Task、有限资源、独立 runtime、L2 外部 API 禁用等硬约束。

## 复用的既有 CFL

| 层 | 默认复用 |
| --- | --- |
| P | `../p-cfl` |
| V | `../value/cmd/l0-v-test-execution-cfl`, `../value/cmd/l1-v-evaluator-cfl`, `../value/cmd/l2-v-human-validate-cfl` |
| R | `../r-cfl/cmd/l0-r-hash-chain-cfl`, `../r-cfl/cmd/l0-r-protocol-validate-cfl` |
| S | `../s-cfl/cmd/l1-s-resource-policy-cfl`, `../s-cfl/cmd/l1-s-lifecycle-death-cfl`, `../s-cfl/cmd/l1-s-inbox-router-cfl` |

运行时状态中的 Pattern/Value/Relation/State 引用分别以 `p-cfl:`, `v-cfl:`, `r-cfl:`, `s-cfl:` 前缀记录。

## CLI

`evolutionctl` 从 stdin 读取 JSON 请求，向 stdout 输出 JSON 响应。状态默认写入 `.evolution-state/<ecosystem_id>.json`，也可用 `state_dir` 指定。

```bash
go run ./cmd/evolutionctl < examples/bootstrap.json
```

常用 `action`：

- `bootstrap`
- `register_direction`
- `submit_task`
- `submit_solution`
- `runtime_tick`
- `send_message`
- `advance_stage`
- `tick`
- `escalate`
- `audit`
- `status`

`claim_task` 仅保留为非排他的 observe 兼容动作；`settle_task` 已废弃，因为新版 PRD 要求 V CFL 由运行时真实执行。

## 当前边界

- 当前产品形态仅研究员侧：研究员提交 Task，reward 配额由全局资源池结算；外部用户形态仍是 future/open option，但资源入口统一叫“用户注入新资源”。
- 不引入 LangChain / AutoGen / CrewAI / Ray / Redis / K8s 等外部 AI 或调度框架。
- L0/L1 可把外部 API token 当作种子内部资源；生态基础设施本身不依赖外部 AI 编排。
- 波 4-6（长期多方向竞争、L0→L1→L2 自治推进、完全切断外部 API 后的下一代 AI 研发）仍需在本 MVP 闭环上继续迭代。
