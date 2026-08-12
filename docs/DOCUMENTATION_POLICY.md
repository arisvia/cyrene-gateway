# Documentation Policy

## Source of truth 顺序

发生冲突时按以下优先级处理：

1. `docs/ARCHITECTURE_DECISIONS.md`
2. 当前 work item 的 spec
3. `progress.json`
4. `docs/ACTIVE_PLAN.md`
5. 审计文档
6. README
7. 历史归档

历史归档只用于追溯，不得覆盖当前决策。

## 文档职责

- `progress.json`：机器可读状态，只记录活动 work items、状态、摘要和验证结果
- `ACTIVE_PLAN.md`：当前执行顺序和禁止事项
- `ARCHITECTURE_DECISIONS.md`：当前有效且必须遵守的架构决策
- `DOMAIN_MODEL.md`：统一术语与边界
- `API_CONTRACT_DIRECTION.md`：Phase 37-38 API 重构方向
- `TEST_STRATEGY.md`：测试与发布门禁
- `ROADMAP.md`：当前阶段之后的候选方向
- `HANDOFF.md`：最近一次跨 Session 交接
- `SCHEDULED_AGENT_PROMPT.md`：无人值守每日 Session 的环境初始化、执行和提交协议
- 审计文档：问题证据与整改建议
- 历史归档：只读

## 控制体积

- `progress.json` 保持在 10 KB 内
- `AGENTS.md` 保持在 5 KB 内
- 已完成 work item 只保留一行 summary 和 verification
- 详细设计进入独立 spec，不塞进 progress notes
- 每完成一个大阶段，归档过期审计和临时计划
