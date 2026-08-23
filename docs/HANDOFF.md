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
- 当前工作项：37D
- 已完成：
  - 实施并验证 Work Item 37A（安全边界与凭据脱敏）。
  - 实施并验证 Work Item 37B（统一 Upstream Executor 与错误分类）。
  - 实施并验证 Work Item 37C：
    - 实现遵循标准规范的 `SSEReader` 与 `FormatSSEEvent`，支持多行 data 聚合、注释跳过及自定义 Event/ID 提取。
    - 添加 `RequestSizeLimiter` 中间件，对 JSON 请求体（10MB）与 Multipart 上传（50MB）设置硬上限。
    - 补充 37C 专项测试并全量 `go test -race ./...` 通过。
- 尚未完成：Phase 37D（网络与文件安全）、37F（Database V2）、37E（发布门禁）。
- 环境状态：Go 1.26.1、Node.js 与 webui 均正常。
- 下一步：按照 `docs/WORK_ITEMS.md` 实施 37D（SSRF 防护、Panel 安全下载与解压、原子配置写入）。
