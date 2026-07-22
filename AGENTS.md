# AGENTS.md — Cyrene Gateway 开发指南

本文件为 AI Agent 在后续 session 中继续开发此项目的上下文文档。

## 项目概述

Cyrene Gateway 是一个 Go 语言实现的高性能 AI 代理网关，从 9router（Next.js）重构而来。
提供 OpenAI 兼容的统一 API 端点，将流量路由到多个上游 AI Provider。

## 架构决策

### 硬性约束（不可违反）

- **语言**: Go 1.26+，纯 Go 实现（CGO_ENABLED=0）
- **数据库**: SQLite（modernc.org/sqlite，纯 Go 驱动，非 mattn/go-sqlite3）
- **日志**: 标准库 log/slog（JSON handler）
- **项目结构**: cmd/gateway（入口）+ internal/（所有业务逻辑）
- **依赖原则**: 优先标准库，引入第三方需在 commit message 说明理由
- **DB 访问**: 所有 DB 操作通过 internal/db 方法，不在 handler 里直接写 SQL
- **Module path**: 不修改 go.mod 的 module path（github.com/arisvia/cyrene-gateway）

### 软性约束（满足触发条件时可调整）

- **HTTP 路由**: 默认 net/http + Go 1.22+ ServeMux（method+path 路由）；若中间件链超过 5 层可引入 chi（仅路由+中间件）
- **数据库**: 默认 SQLite；若需多实例部署，可通过 database/sql 接口替换为 Postgres
- **面板**: 默认 templates/index.html 单文件；若复杂度超出单文件承载能力，可拆分为独立前端目录（但仍由 Go 二进制 embed 或 serve）

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

- **主参考**: decolua/9router（Next.js 原版，`--depth 1` clone 到 /data/workspace/9router）
- **增强参考**: Vanszs/VansRouter（loop guard、termination prompt 等增强，按需 clone 到 /data/workspace/VansRouter）
- 重点参考目录：`open-sse/services/`（fallback/credential）、`open-sse/config/`（error rules）、`open-sse/handlers/`（chat core）

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

- **开发期（Phase 2-9）**：9router 和 VansRouter **都参考**
  - 9router：核心架构、路由逻辑、provider 定义的权威来源
  - VansRouter：已做的增强（loop guard、termination prompt、bug fix）直接借鉴
  - 9router clone 到 /data/workspace/9router（若不存在则每轮 session 开头 clone）
  - VansRouter 仅在当前 phase 明确需要时按需 clone 到 /data/workspace/VansRouter
- **维护期（Phase 10）**：只定期 diff 9router（主上游）
  - VansRouter 是 9router 的 fork，底层 90%+ 相同，定期 diff 会重复运算
  - 仅在 Issue 指定或其独有增强相关时按需查看
- 上游 commit hash 记录在 `progress.json` 的 `upstream_commits` 字段
- 新增上游参考：往 `upstream_commits` map 加一条即可

## Issue 驱动开发

用户可以随时在 GitHub Issues 发布需求：
- `bug` label → 维护模式优先修复
- `enhancement` label → 功能增强（用户的点子）
- `upstream` label → 标记从上游借鉴的改动
- 维护模式每次触发都会检查 open issues 并处理

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

Go 版本：
```bash
# 平台内置 Go 可能低于 1.26，需升级（必须先删旧版，否则 runtime 文件冲突）：
rm -rf /usr/local/go && curl -sL https://go.dev/dl/go1.26.2.linux-amd64.tar.gz | tar -C /usr/local -xz
go version  # 应输出 go1.26.2
```

平台内置工具（无需手动预装）：
git, curl, wget, jq, sqlite3, make, gcc, rg, tar, unzip, python3, node

## 不要做的事

- 不要修改 go.mod 的 module path（github.com/arisvia/cyrene-gateway）
- 不要引入 CGO 依赖
- 不要删除或重写已完成的 phase 代码（除非有明确 bug）
- 不要一次做多个 phase
- 不要 force push（除非修正 commit 作者等特殊情况）
- 不要跳过测试直接提交
- 遇到阻塞时不要强行推进，记录到 progress.json 的 notes 字段并停止
