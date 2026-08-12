# Cyrene Gateway Agent Guide

本文只保留后续开发所需的活动上下文。完整历史见 `docs/AGENTS-full-before-phase37.md` 与 `docs/progress-full-before-phase37.json`。

## 项目

Cyrene Gateway 是 Go 1.26+ 的 AI 代理网关，提供 OpenAI-compatible API，并将请求路由到多个 Provider。管理面板为 Vue 3 + Vite + TypeScript，通过 `go:embed` 嵌入单二进制。

## 当前工作

按以下顺序阅读：

1. `progress.json`
2. `docs/HANDOFF.md`
3. `docs/WORK_ITEMS.md` 中当前 work item
4. `docs/ARCHITECTURE_DECISIONS.md`
5. 与任务相关的审计、领域模型、API 和测试文档

当前 Phase 37 是公开部署前的安全与代理可靠性门禁；Phase 38 重构 WebUI Provider/Connection 工作流。不要在这两阶段完成前新增 Provider 或堆视觉特效。

## 硬约束

- Go 1.26+，`CGO_ENABLED=0`
- SQLite 使用 `modernc.org/sqlite`
- 日志使用 `log/slog`
- 业务逻辑位于 `internal/`
- DB 操作只通过 `internal/db`
- 不修改 module path `github.com/arisvia/cyrene-gateway`
- 运行数据固定为 `~/.cyrene-gateway/`
- 项目尚未投产，不保留旧 API、旧字段、旧数据库或旧 WebUI 的兼容层
- Provider 与 Connection 必须作为不同领域概念
- 持久化模型不得直接作为管理 API 响应，secret 必须脱敏
- single/combo/messages/tester 必须共用 upstream executor

## 目录

```text
cmd/gateway             入口与 graceful shutdown
internal/config          配置
internal/db              SQLite repositories
internal/model           内部模型
internal/provider        registry、transport、routing、OAuth
internal/handler         HTTP API
internal/middleware      中间件
internal/translator      OpenAI/Anthropic/Gemini 转换
internal/usage           用量与配额
internal/media           Media providers
internal/cli             外部 CLI 配置
internal/mitm            MITM
internal/tunnel          Tunnel/Tailscale
webui/src                Vue 管理面板
docs                     活动设计、审计与历史归档
```

## 每个子阶段的工作流

1. 只完成 `progress.json.current_work_item`，遵守依赖关系和 `docs/WORK_ITEMS.md` 的验收标准。
2. 修改实现、测试、API contract 与文档，不留临时兼容代码。
3. 后端执行：

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

4. 前端变更执行：

```bash
cd webui
npm ci
npm test
npm run build
```

5. 涉及 WebUI 时必须启动应用并进行浏览器走查，覆盖 dark/light、375px、空/加载/错误/长文本状态及完整 Provider 添加流程。
6. 只有 work item 验收标准全部通过才能标记 done；Session 结束更新 `docs/HANDOFF.md`。所有 work item 完成后才能标记 phase done。

## 发布门禁

- Phase 37 未完成，不允许公网部署。
- Phase 38 未完成，不宣称管理面板产品可用。
- 任何 secret 出现在列表或详情响应中均视为阻断问题。
- 任何 race、SSE 截断伪装成功、Combo 协议分叉均视为阻断问题。
