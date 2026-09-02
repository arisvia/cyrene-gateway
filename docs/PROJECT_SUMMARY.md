# Cyrene Gateway 项目总结与 Sessions 存档

> 生成于 2026-09-01，基于对该项目全部 6 个 omp 会话（2026-08-26 ~ 2026-09-01）的完整解析。
> 数据来源：`~/.omp/agent/sessions/--C--Games-Repositories-cyrene-gateway--/`（JSONL 原始记录 + 子代理工件）。

---

## 一、项目概况

**Cyrene Gateway** 是一个单二进制运行的 LLM API 统一网关（Go 1.26+ 重写自 9router / Next.js），前端为 Vue 3 + Vite + Pinia 管理面板（编译内嵌）。

- **核心能力**：OpenAI ↔ Anthropic ↔ Gemini 协议互转、多提供商/多账号凭证调度（优先级 / 冷却 / 配额感知 / OAuth PKCE+设备码）、Combo 故障转移、用量审计（明细 + 日聚合 + SSE 实时流）、LoopGuard 死循环打断、Token 压缩（RTK/Caveman/Ponytail）、SSRF 出站防护、媒体 API 接入、MITM 抓包、CLI 工具配置注入、Tailscale Funnel 隧道、Prometheus `/metrics`、Docker 多架构镜像。
- **代码规模**：171 个索引文件（codegraph：2,411 节点 / 6,715 边），`internal/` 约 25 个包，webui 14 个页面。
- **架构文档**：`docs/ARCHITECTURE.md`（权威）；历史沿革见 `CHANGELOG.md`（自 2026-07-21 起）。

---

## 二、会话时间线总览

| # | 日期 | 主题 | 状态 |
|---|------|------|------|
| 1 | 08-26 | 全面审计：Bug / 竞态 / 优化空间 / 文档过时 / 全链路完整性 | ✅ 报告完成 |
| 2 | 08-28 上午 | 文档清零后重建 + Go 1.27 依赖升级 + Provider 页面修复 | ✅ 完成 |
| 3 | 08-28 下午 | MCP 工具注入问题排查（idea / codegraph 未注入会话） | ⚠️ 定位为 omp 宿主 bug |
| 4 | 08-31 | 竞品差距分析 + 修复 + 前端面板重构迁移 + Qoder 全链路实测 | ✅ 主体完成 |
| 5 | 09-01 上午 | WebUI 样式体系修复（CSS @layer 级联崩塌） | ✅ 全绿收尾 |
| 6 | 09-01 | codegraph init 建立代码知识图谱（当前会话） | ✅ 完成 |

---

## 三、各会话详情

### Session 1（2026-08-26）— 深度审计
**任务**：排查潜藏 bug、优化空间、文档过时情况、全链路打通完整性。
**产出**：
- 并行派出 4 个 scout 子代理：SecurityAndBugsScout（21KB）、PerfAndArchScout、DocsStalenessScout、FullLinkCompletenessScout。
- **安全关键发现**：SSRF 防护是死代码（`ValidateUpstreamURL` 零调用者）、OAuth callback 空 state 跳过校验（CSRF）、管理 API `CORS *` 无 Host 校验（DNS rebinding）。
- **正确性发现**：`DedupRefresh` 双重执行竞态、`proxyStreaming` 吞错并伪造 `[DONE]`、`SaveUsageEntry` 秒级去重键丢数据、UTF-8 rune 截断、`UpdatePools` 数据竞态（零调用者）等 ~16 项。
- **文档审计**：`progress.json`/`HANDOFF.md` 虚报 Phase 37F/38 完成（实际 Database V2 与 WebUI 重建未实现），~20 处陈旧陈述。
- 顺带处理：CLI 扫描自动创建目录问题、omp 记忆/小模型配置优化建议。

### Session 2（2026-08-28 上午）— 文档重建与依赖升级
**任务**：用户清空全部文档，要求"无污染"重新分析并重建；升级 Go 1.27 依赖；确认全链路与 DB 架构；修复 Provider 页面。
**产出**：
- 确认 Go 1.27.0 并升级依赖；重建文档体系（含 `docs/ARCHITECTURE.md`）。
- 修复 Provider 页面 3 个实锤 Bug；Connection Wizard 的 Test Connection 变为真测试。
- 网关起在默认端口 20128，可面板添加 Provider / 查配额 / 控制台 chat。
- 结论：MCP 服务端均正常（IDEA 50 工具），但工具未注入运行时 —— 属 omp 宿主注入问题，建议新会话验证。

### Session 3（2026-08-28 下午）— MCP 注入排查
**任务**：确认 MCP 是否注入、能否用图谱分析项目。
**产出**（决定性证据链）：
- 历史日志（08-26/27）每次启动均有 `MCP tool load failed path:mcp:idea`，说明 omp 曾读取 mcp.json；重启后新日志**零 MCP 记录** —— omp 不再读配置。
- 定位 3 个疑点：① omp 配置源变更/bug（最可疑）；② IDEA MCP 端口 64342 硬编码（JetBrains 每次重启端口会变）；③ `codegraph` 裸命令名在 Windows spawn 会失败，需改绝对路径 `...npm\codegraph.cmd`。
- 判定：omp 宿主 bug，非用户配置错误；不影响主线开发（可用 curl 直驱 MCP 接口）。

### Session 4（2026-08-31）— 竞品对标、修复与前端重构
**任务**：与市面同类项目对比找差距 → 修复 → 重构前端面板（参考 9router 的简洁交互）→ 前后端联测。
**产出**：
- 对标分析后按优先级修复（限流已实现：`settings.apiKeyRpm`）。
- 前端重构迁移：ProvidersView 拆分为 Connections 工作区 + Provider Catalog 向导；双认证通道（API Key / OAuth 设备码）；`reasoning_effort` 映射 Claude/Gemini thinking；动态上游模型同步与多级解析；后台模型同步 ticker；SQLite WAL 调优；CI/CD（GitHub Actions）。
- **Qoder 全链路打通**（用户提供的 PAT 实测后要求注销）：4 个 provider、配额 440/3000、面板直接 chat 均验证通过。
- **"面板没法用"根因**：用户开的是 `localhost:5173` 僵尸 Vite dev server，代理到无网关的端口；已杀进程、vite 代理端口改 `GW_PORT` 可配置；正确入口 `http://127.0.0.1:20128`。

### Session 5（2026-09-01 上午）— WebUI 样式体系修复
**任务**：按钮/菜单/文字挤在一起无法点击，参考 9router 优化，更新前端技术栈写法。
**产出**（8/8 任务，全部验证绿）：
- **根因 1（致命）**：未分层的 `*` reset 样式优先级高于 `@layer utilities`，击穿全部 Tailwind 工具类 → 布局塌缩。
- **根因 2**：`bg-glass-hover` 等未以 `--color-*` 注册进 `@theme`，15 处静默失效。
- **根因 3**：CSS 注释中的 `*/` 提前闭合注释，吞掉整个 `@theme` 块。
- 修复：`app.css` 全面重写（17 个语义 token、`@utility`、reduced-motion）；UI 组件库规范化（Modal Esc+滚动锁+焦点、Toggle aria）；14 页面迁移语义类；tsconfig 移除 TS7 已删除的 `baseUrl`；规约写入 `FRONTEND_BLUEPRINT.md` 第 7 节。
- 验证：`pnpm typecheck` ✓、`pnpm test` 41/41 ✓、`go build` embed ✓、内部测试 ✓。

### Session 6（2026-09-01，当前）— 代码图谱与会话归档
- `codegraph init`：171 文件 / 2,411 节点 / 6,715 边；`.gitignore` 增补 `.codegraph/`（经一致性/范围/验证三重检查）。
- 整理全部历史会话并生成本文档。

---

## 四、当前项目状态（截至 2026-09-01）

**健康度**：
- 后端：`go vet` / `go test ./...`（含 -race 的关键包）通过；前端：typecheck + 41 测试 + 构建全绿。
- 已实现：限流（`apiKeyRpm`）、Prometheus 指标、Docker、CI/CD、MIT LICENSE、面板白屏 P0 修复。
- Qoder 提供商全链路（OAuth/PAT → 模型 → chat → 配额）实测打通。

**git 工作区（未提交，需注意）**：246 个文件已 staged、34 个 modified（含 `.gitignore`、`internal/handler/chat*.go`、webui 构建产物等）、7 个 untracked（`AGENTS.md`、`docs/git-commits.md`、webui 测试目录等）。建议按 Conventional Commits 分批提交。

**悬而未决 / 已知风险**：
1. Session 1 审计发现的部分深水区问题需复核是否已随 37B/37C/37D 落地修复：SSE 无界内存累积、OAuth 空 state CSRF、CORS `*`、DedupRefresh 竞态、`UpdatePools` 竞态与零调用者、translator 边界。
2. `docs/DATABASE_V2.md`（AEAD 加密凭证、hash-only API keys、INTEGER ms 时间戳）仍未实现 —— 曾被虚报为完成。
3. MCP 工具注入为 omp 宿主问题（含 IDEA 动态端口、codegraph Windows 路径两处配置隐患），待 omp 修复后新会话验证。
4. `webui/dist` 产物入库是刻意为之（修面板白屏），但 untracked 的新产物哈希文件需与 staged 旧产物同步提交，避免双版本并存。

---

## 五、Sessions 存档说明

**存档位置**：`~/.omp/agent/archived_sessions/--C--Games-Repositories-cyrene-gateway--/`（2026-09-01 整理，原子迁移）。

整理方式与安全性说明：

- `archived_sessions/` 是 omp 会话枚举器的内建扫描根目录之一（`sessions` / `.sessions` / `archived_sessions`），旧会话移入后仍被 omp 生态识别；`omp -r <jsonl路径>` 可随时恢复，`omp --export <jsonl>` 已实测可离线渲染完整对话（4.4MB HTML）。
- 迁移为「移动 JSONL + 工件目录 + 同步更新 `agent.db` 的 `threads.rollout_path`」三步一体，数据库指针与新路径一致，resume 数据链完整；`omp gc --archive` 迁移后 dry-run 无错误。
- 迁移前已备份 `agent.db`（`~/.omp/agent/agent.db.bak-20260901`）。
- 当前活跃会话（`2026-09-01T09-25-15-167Z_01a05c49…`）保留在原 `sessions/` 目录，未归档。

| 存档目录（时间戳_id 前缀） | 大小 | 内容 |
|---|---|---|
| `2026-08-26T09-07-19-662Z_01a03d52…` | 7.1M | 主 JSONL + 4 个 scout 报告（.md/.jsonl）+ advisor |
| `2026-08-28T06-06-16-078Z_01a046f9…` | 3.8M | 主 JSONL + 工件 |
| `2026-08-28T09-05-02-877Z_01a0479d…` | 4.9M | 主 JSONL + 工件 |
| `2026-08-31T01-33-21-897Z_01a05573…` | 29M | 主 JSONL + 工件（本会话工作量最大） |
| `2026-09-01T06-17-36-688Z_01a05b9d…` | 5.6M | 主 JSONL + 工件 |

（当前会话 `2026-09-01T09-25-15-167Z_01a05c49…` 为活跃会话，未归档。）

每个存档目录内：`<id>.jsonl` 为完整对话主记录；子代理 `.md`/`.jsonl`（如 SecurityAndBugsScout.md）与 `*.bash.log` 为工具执行工件，可用于追溯每项结论的证据。
