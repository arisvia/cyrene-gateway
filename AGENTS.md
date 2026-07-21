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
- Phase 9 完成 → v0.9.0
- 最终稳定 → v1.0.0

打 tag 会触发 GitHub Actions 创建 Release（含 5 平台二进制）。

## Dashboard 面板设计（Phase 5）

三层降级策略：
1. `-dashboard /path/to/ui` → 用户指定的本地前端目录（最高优先级）
2. 内置 embed `templates/index.html`（Vue3 + TailwindCSS via CDN）→ 零配置兜底
3. `-panel-url` → 可选，拉取远程更新版面板（默认指向本仓库 raw 文件）

面板是单 HTML 文件，随主仓库维护在 `templates/index.html`，不需要独立前端仓库。

CLI 参数（全部有默认值）：
```
-host 0.0.0.0       # 监听地址
-port 20128         # 端口
-db data.sqlite     # 数据库路径
-dashboard ""       # 本地面板路径（空=使用内置）
-panel-url https://raw.githubusercontent.com/arisvia/cyrene-gateway/main/templates/index.html
-secret ""          # Dashboard 访问密码
```
环境变量 CYRENE_HOST / CYRENE_PORT 等同样支持，flag 优先于 env。

## 维护模式（Phase 9 完成后）

当所有开发 phase（1-9）均为 done 时，后续定时触发进入维护模式：
1. 检查 GitHub Issues（bug 报告、feature request）
2. `go build ./... && go test ./...` 验证项目健康
3. 审查 dependabot PRs，安全则合并
4. 修复 issue → commit → push → 打 patch tag（v1.0.1 等）
5. **上游同步**：对比 `progress.json` 中 `upstream_commits` 的 hash
   - 用 GitHub API 查询新提交：`curl -s "https://api.github.com/repos/decolua/9router/commits?since=<date>"`
   - 重点关注 `open-sse/providers/registry/`（新 provider、baseUrl 变更）
   - 重点关注 `open-sse/config/`（模型映射、能力变更）
   - 有值得借鉴的改动 → 移植到 Go → commit → push
   - 更新 `upstream_commits` hash
6. 无需操作时报告 "no pending work" 并正常退出

Phase 10 永远不会被标记为 done——它是持续运行的守护状态。

## 参考项目使用策略

- **开发期（Phase 2-9）**：按需 `git clone --depth 1` 到 /tmp，用完即弃
- **维护期（Phase 10）**：不 clone，用 GitHub API 对比 commit 差异
- **不需要每次 session 都 clone 参考项目**——只在当前 phase 任务明确需要时拉取
- 上游 commit hash 记录在 `progress.json` 的 `upstream_commits` 字段

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
