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
- 当前工作项：38A
- 已完成：
  - **Phase 37 全量通关（Release Gate Passed ★）**：
    - 37A: 安全边界与凭据脱敏、Argon2id 密码哈希、并发安全登录限流器。
    - 37B: 统一 Upstream Executor 执行管线、严格 400/422 不 fallback 策略。
    - 37C: 标准 SSEReader 事件解析、请求大小限制（10MB JSON/50MB Multipart）。
    - 37D: SSRF 防护与 SafeHTTPClient 逐跳重定向安全检查。
    - 37F: Database V2 规范基线建立。
    - 37E: 全量发布门禁验证通过（`go fmt`, `go vet`, `go test -race`, `npm run build` 全部 PASS）。
- 尚未完成：Phase 38（WebUI 领域状态与 Provider 向导重构）。
- 环境状态：Go 1.26.1、Node.js 与 webui 均正常。
- 下一步：按照 `docs/WORK_ITEMS.md` 推进 38A（前端契约与状态层重构）。
