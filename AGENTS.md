# AGENTS.md — Cyrene Gateway 开发指南

本文件为 AI Agent 在后续 session 中继续开发此项目的上下文文档。

## 项目概述

Cyrene Gateway 是一个 Go 语言实现的高性能 AI 代理网关，从 9router（Next.js）重构而来。
提供 OpenAI 兼容的统一 API 端点，将流量路由到多个上游 AI Provider。

## 架构决策（已锁定，不要更改）

- **语言**: Go 1.22+，纯 Go 实现（CGO_ENABLED=0）
- **数据库**: SQLite（modernc.org/sqlite，纯 Go 驱动，非 mattn/go-sqlite3）
- **HTTP**: 标准库 net/http + Go 1.22 ServeMux（不引入 gin/echo/chi）
- **日志**: 标准库 log/slog（JSON handler）
- **项目结构**: cmd/gateway（入口）+ internal/（所有业务逻辑）
- **无前端**: 本项目只做后端网关，Dashboard UI 不在范围内

## 目录结构

```
cmd/gateway/main.go          # 入口，graceful shutdown
internal/config/             # CLI flags + env 配置
internal/db/                 # SQLite 持久层（所有 repository）
internal/model/              # 领域模型（struct 定义）
internal/provider/           # Provider 注册表 + Model 解析
internal/handler/            # HTTP handlers（API surface）
internal/middleware/         # 中间件（预留）
internal/usage/              # 用量追踪（预留）
.github/workflows/build.yml  # CI 多平台构建
progress.json                # 重构进度追踪（核心文件）
schema.sql                   # 数据库 schema 参考
```

## 开发规范

- 每个 phase 完成后：`go fmt ./...` → `go build ./...` → commit → push → 更新 progress.json
- Commit 格式：`feat: Phase N - 描述` 或 `fix: 描述`
- 不引入不必要的第三方依赖，优先标准库
- 所有 DB 操作通过 internal/db 的方法，不在 handler 里直接写 SQL
- Provider 注册表是静态配置，新增 provider 在 internal/provider/registry.go 添加

## 参考源码

- **主参考**: decolua/9router（Next.js 原版，`--depth 1` clone 到 /tmp 分析）
- **增强参考**: Vanszs/VansRouter（loop guard、termination prompt 等增强）
- 重点参考目录：`open-sse/`（路由核心）、`src/sse/`（请求处理）、`src/lib/db/`（持久层）

## progress.json 使用规则

- 位于仓库根目录，是跨 session 的唯一进度来源
- `phases[].status`: "pending" | "done"
- `current_phase`: 指向下一个要做的 phase ID
- 每次 session 只完成一个 phase，完成后更新 status 和 current_phase
- 必须随代码一起 commit 并 push

## 版本规划

- Phase 1 完成 → v0.1.0
- Phase 2 完成 → v0.2.0
- ...
- Phase 8 完成 → v0.8.0
- 最终稳定 → v1.0.0

打 tag 会触发 GitHub Actions 创建 Release（含 5 平台二进制）。

## 环境配置

启动时加载 `/data/.env`：
```
GITHUB_TOKEN=<pat>
GITHUB_REPO=arisvia/cyrene-gateway
GITHUB_USER=arisvia
GITHUB_EMAIL=160387885+arisvia@users.noreply.github.com
```

Git 配置：
```bash
git config user.name "$GITHUB_USER"
git config user.email "$GITHUB_EMAIL"
```

## 不要做的事

- 不要修改 go.mod 的 module path（github.com/arisvia/cyrene-gateway）
- 不要引入 CGO 依赖
- 不要添加前端代码
- 不要删除或重写已完成的 phase 代码（除非有明确 bug）
- 不要一次做多个 phase
- 不要 force push（除非修正 commit 作者等特殊情况）
