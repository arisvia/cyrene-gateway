# Cross-Session Handoff

每个 Agent session 结束前必须更新本文件的“最近一次交接”区块，并保持简短。

## 开始工作

1. 读取 `progress.json` 的 `current_work_item`。
2. 读取该 work item 的 `spec`。
3. 读取 `docs/ARCHITECTURE_DECISIONS.md` 与相关审计。
4. 检查工作区是否有未提交修改。
5. 先运行该模块现有测试，记录 baseline。
6. 只处理一个 work item，不顺手开启下一项。

## 完成工作

- 更新实现和测试
- 更新 API contract 或产品文档
- 执行 work item 的验证命令
- 在 `progress.json` 更新 status、summary、verification
- 将 `current_work_item` 指向下一个 pending work item
- 在下方记录修改、验证、剩余风险和下一步

## 阻塞规则

如果环境缺少 Go、Node、浏览器或网络依赖，不得把 work item 标记 done。将状态设为 `blocked`，记录准确原因和恢复命令，不得声称测试通过。

## 最近一次交接

- 日期：2026-08-23
- 当前工作项：38B
- 已完成：
  - **Phase 37 全量通关（Release Gate Passed ★）**
  - **Work Item 38A 通关**：
    - 在前端引入强类型 `ConnectionDTO`、`ConnectionDataDTO` 与 `ProviderCatalogItem` 契约定义。
    - 补充前端 Contract 单测，验证脱敏与无 Secret 传输。
    - 前端 Vitest 单测与生产构建全部通过。
- 尚未完成：Phase 38B（Provider 向导重构）及后续 UI 工作项。
- 环境状态：Go 1.26.1、Node.js 与 webui 均正常。
- 下一步：按照 `docs/WORK_ITEMS.md` 推进 38B。
