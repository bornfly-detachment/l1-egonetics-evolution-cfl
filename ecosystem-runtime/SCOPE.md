# Evolution scope

## In scope now

- PRD 波 1-3（5-18 修正版）的可执行闭环：独立 seed runtime、R message bus、open Task、多解并存、V 真实执行、两档奖励、死亡、escalation、audit。
- stdlib-only Go module，保持 H3 零外部污染。
- 复用已有 P/R/V/S CFL 的注册、引用、边界与后续接入点。

## Out of scope now

- 单个 AI 种子的内部架构/算法/数据设计。
- 外部 API provider 调用实现；这里仅把 `api_token` 建模为稀缺资源。
- 外部用户产品化 / 用户验收；当前资源入口统一建模为 `resource_injection`，不是商业支付模型。
- 专用 V 维度构建算法；运行时必须把评价维度交给 V，不写死 ROI 清单。
- 波 4-6 的长期自治实验和“下一代 AI 涌现”判准。

## Hard constraints preserved

- E 不是 PRVSE 第五层，不是新 CFL。
- bornfly 只 boot + escalation，不是 runtime operator。
- 所有 Task 默认公开给全部存活种子，且永不关闭。
- 取消 claim 排他；任何活种子任何时候可提交解。
- 同一研究方向固定 3 种子：两极端一中庸。
- 种子是独立 runtime，不是中央 state struct 字段；死亡会终止 runtime。
- 种子间自由通信不受限制，生态层 R-CFL 是 message bus。
- L2 种子禁止外部 API 智能；违反即取消资格。
- 可消耗资源必须有限、可计量、可扣减。
