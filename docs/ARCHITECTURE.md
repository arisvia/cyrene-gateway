# Cyrene Gateway 架构设计文档

本文档描述 Cyrene Gateway 的系统架构、核心组件职责、请求生命周期与数据流，供二次开发与维护参考。

---

## 1. 架构总览

Cyrene Gateway 是一个单二进制运行的 LLM API 统一网关，核心职责包括协议转换、多账号/多提供商凭证调度、自动故障转移与配额管理、用量审计以及配套的开发者/管理工具（面板、MITM、CLI 适配器、Tailscale 隧道）。

```
+-------------------------------------------------------------------------------+
|                                  客户端                                       |
|  - CLI 工具 (Claude Code / Codex / OpenCode / Cline / Copilot CLI 等)          |
|  - 业务后端 / 开发者应用 (OpenAI / Anthropic SDK)                             |
|  - Web 管理面板 (SPA, 浏览器)                                                 |
+---------------------------------------+---------------------------------------+
                                        | HTTP / HTTPS / SSE
                                        v
+-------------------------------------------------------------------------------+
|                            中间件管道 (Middleware)                            |
|  Recovery -> Logging -> RequestSizeLimiter -> CORS -> APIKeyAuth -> SessionAuth |
+---------------------------------------+---------------------------------------+
                                        |
       +--------------------------------+--------------------------------+
       |                                                                 |
       v /v1/* 业务请求                                                  v /api/* 管理请求
+------------------------------+                                  +------------------------------+
|       业务处理器 Handler     |                                  |        管理处理器 Handler    |
|  - /v1/chat/completions      |                                  |  - /api/providers (CRUD/测试)|
|  - /v1/messages (Anthropic)  |                                  |  - /api/combos (模型组合)    |
|  - /v1/embeddings            |                                  |  - /api/oauth/* (PKCE/设备码)|
|  - /v1/models (聚合目录)     |                                  |  - /api/usage/* (统计/流式)  |
+--------------+---------------+                                  |  - /api/cli-tools / tunnel   |
               |                                                  |  - /api/mitm/* (TLS 代理)    |
               v                                                  +--------------+---------------+
+---------------------------------------------------------------+                |
|                    Provider 调度与执行层                      |                |
|  - 模型与组合解析 (ResolveModel / Combos)                     |                |
|  - 凭证与节点选择 (SelectCredentialWithQuota / Priority / 锁) |                |
|  - Token 节省 (RTK / Caveman / Ponytail)                      |                |
|  - 死循环打断 (LoopGuard)                                     |                |
|  - 出站适配与重试 (Transport / SSRF 防护 / ProxyPool)         |                |
|  - 协议与流式互转 (Translator / SSE Stream / ExtractUsage)    |                |
+------------------------------+--------------------------------+                |
                               |                                                 |
                               +------------------------+------------------------+
                                                        |
                                                        v
                                       +----------------------------------+
                                       |      存储层 (SQLite / WAL)       |
                                       |  - providerConnections           |
                                       |  - settings / kv                 |
                                       |  - combos / proxyPools / apiKeys |
                                       |  - usageHistory / usageDaily     |
                                       +----------------------------------+
```

---

## 2. 核心模块与职责划分

| 模块路径 | 关键类型 / 入口 | 职责 |
|---|---|---|
| `cmd/gateway/` | `main.go` | 服务启动入口，加载配置、初始化 DB、启动后台模型同步协程、优雅停机（SIGINT/SIGTERM）。 |
| `internal/config/` | `Config`, `Load()` | 解析命令行参数与 `CYRENE_*` 环境变量，设置默认路径 `~/.cyrene-gateway/`。 |
| `internal/db/` | `DB`, `Open()` | 基于 `modernc.org/sqlite`（纯 Go、零 CGo），单连接 WAL 模式，管理表结构迁移、连接、组合、密钥、日用量聚合事务。 |
| `internal/middleware/` | `Chain`, `Recovery`, `APIKeyAuth`, `DashboardAuth` | 全局中间件链：Panic 恢复、请求大小限制（JSON 10MB / Multipart 50MB）、CORS、API Key 签名与白名单校验、管理端会话鉴权。 |
| `internal/auth/` | `GenerateAPIKey`, `HashPassword`, `LoginLimiter` | HMAC 签名与验签、Argon2id 密码哈希（兼容旧 HMAC 自动升级）、防爆破指数锁定（30s~30m）。 |
| `internal/provider/` | `Registry`, `SelectCredentialWithQuota`, `SafeHTTPClient` | 静态提供商元数据注册表、OAuth 授权流程（PKCE 与设备码）、Token 刷新防抖、出站 SSRF 防护与代理池调度。 |
| `internal/model/` | `ModelMetadata`, `FetchModels`, `Catalog` | 模型 DTO 定义、上游模型列表动态拉取与 24 小时缓存、OpenAI 格式模型聚合目录。 |
| `internal/translator/` | `TranslateRequest`, `TranslateResponse` | OpenAI ↔ Anthropic ↔ Gemini 之间的请求体、响应体与 SSE 流式事件的无损双向转换。 |
| `internal/usage/` | `ExtractFromOpenAI`, `ExtractFromClaude` | 从多协议的响应体与 SSE 事件中抽取 input / output / cached / reasoning token 并做规范化记账。 |
| `internal/loopguard/` | `DetectLoop` | 对话历史死循环检测：识别重复工具调用或纯文本复读，在单模型和 Combo 流程中注入打断提示。 |
| `internal/media/` | `Client`, `GetConfig` | 媒体 API 接入适配（Embeddings / STT / TTS / Image / Video / Web Search）。 |
| `internal/mitm/` | `Server`, `CertManager`, `DNSManager` | 本地 TLS 拦截代理，生成自签名根 CA，劫持 hosts 抓包排查 CLI 工具请求。仅限本机部署开启。 |
| `internal/tunnel/` | `Manager` | Tailscale 状态探测、安装、登录、启动/关闭 Funnel 隧道暴露到公网。 |
| `internal/cli/` | `Manager`, `Adapter` | 十余款终端 AI 工具（Claude Code、Codex 等）的配置自动注入与重置。 |
| `webui/` | Vue 3 + Vite + Pinia | 响应式管理控制台，通过 `embed.go` 编译内嵌到二进制中。 |

---

## 3. 请求生命周期与数据流

### 3.1 `/v1/chat/completions` 请求生命周期

```
Client
  │  POST /v1/chat/completions (JSON body)
  ▼
[Middleware: Recovery -> Logging -> RequestSizeLimiter -> CORS]
  │
[Middleware: APIKeyAuth]
  ├─ 检查 settings.requireApiKey
  └─ 若开启：提取 Authorization/x-api-key -> HMAC 签名快速验签 -> DB 比对是否激活
  ▼
[Server.handleChatCompletions]
  │  解析 ChatCompletionRequest
  ├─ 判断是否命中 Combo (provider.IsCombo)
  │    ├─ [是] -> Server.handleComboChat: 按 fallback/round-robin 依次迭代备选模型
  │    └─ [否] -> Server.handleSingleModelChat: 处理单个目标模型
  │
  ├─ [LoopGuard 检查]: 检测对话是否存在循环，若有则注入 Loop Hint
  ├─ [TokenSaver 优化]: 依据全局设置注入 RTK / Caveman / Ponytail 压缩指令
  │
  ├─ [模型与提供商解析]: provider.ResolveModel
  │    └─ 解析模型别名 -> 匹配自定义 ProviderNode 或从前缀推断 Provider
  │
  ├─ [凭证选择]: provider.SelectCredentialWithQuota
  │    └─ 过滤冷却中/锁定的连接 -> 按 Priority 排序 -> OAuth 连接优先 -> 校验配额
  │
  ├─ [Token 自动刷新]: tryRefreshToken / tryRefreshCopilotToken
  │    └─ provider.DedupRefresh 保证同一 Token 只有一个并发刷新请求
  │
  ├─ [出站转发]: getHTTPClient (通过 SafeHTTPClient 或 ProxyPool)
  │    ├─ SafeHTTPClient 在 Dial 阶段执行 SSRF 防御，拦截内网/环回/元数据 IP
  │    └─ 依据 ProviderInfo.APIType 与 Translator 进行协议格式转换
  │
  ├─ [响应处理与流式中继]:
  │    ├─ 流式 (stream=true): proxyStreaming 逐 chunk 转发 SSE，解析 [DONE]，提取 usage
  │    └─ 非流式: proxyNonStreaming 读取完整响应，执行格式反向转换
  │
  ├─ [错误与退避机制]:
  │    ├─ 上游返回 429/5xx 时触发 provider.CheckFallbackError
  │    ├─ 设置指数退避冷却 (2s~5min) 或模型级锁定
  │    └─ Combo 模式下自动静默重试下一可用模型
  │
  ▼
[Server.recordUsage]
  │  异步记录 UsageEntry 到 SQLite (usageHistory 表)
  ├─ 事务内原子增量更新 usageDaily 日聚合表
  └─ EventBroadcaster 发布 SSE 实时事件到 /api/usage/stream
  ▼
Client 接收最终响应
```

---

## 4. 核心机制详解

### 4.1 出站安全与 SSRF 防护（`internal/provider/ssrf.go`）
- **解析阶段校验**：`ValidateUpstreamURL` 检查 URL Scheme（仅允许 http/https）以及主机解析出的 IP。
- **拨号阶段校验**：`SafeDialerControl` 自定义 `net.Dialer.Control`，在实际发起 TCP 连接前拦截环回地址（`127.0.0.0/8`、`::1`）、私有网络（RFC 1918）、链路本地地址（`169.254.0.0/16` 包含云元数据）、CGNAT（`100.64.0.0/10`）以及组播地址。
- **重定向安全**：`http.Client.CheckRedirect` 在每次跟随 30x 重定向时重新对目标 URL 执行安全审计，防止通过公网 URL 302 跳转至内网靶机。
- **开发放行**：通过 `-allow-private-networks` 可显式放行，便于开发者在本地连接自建的 Mock 服务或 Ollama。

### 4.2 OAuth 刷新防抖与并发控制（`internal/provider/oauth_flow.go`）
当多个请求同时打入且连接凭证即将过期时，`DedupRefresh` 采用双重检查锁与互斥锁通道：
1. 计算 `provider:token` 粒度的锁；
2. 第一个协程发起远程 Token Refresh 并将新 Token 写回数据库；
3. 后续并发协程等待该刷新任务完成，直接复用刷新的结果，避免向上游 OAuth Provider 发送重复请求导致 Refresh Token 作废。

### 4.3 数据库设计与高性能并发（`internal/db/`）
- **WAL 模式**：通过 PRAGMA `journal_mode(WAL)`、`busy_timeout(5000)`、`synchronous(NORMAL)` 保障高吞吐低延迟。
- **单写连接**：`conn.SetMaxOpenConns(1)` 彻底消除 SQLite 并发写锁竞争（`database is locked`）隐患。
- **日聚合双写**：`SaveUsageEntry` 将逐笔明细写入与 `usageDaily` 的 JSON 增量合并封装在单个事务内，兼顾明细查询与看板大盘统计秒级加载。

---

## 5. 扩展与二次开发指南

### 5.1 新增一个静态 Provider
1. 在 `internal/provider/registry_data.go` 的 `Registry` map 中注册新的 `ProviderInfo`。
2. 配置对应的 `BaseURL`、`APIType`（`openai` / `anthropic` / `gemini`）、`AuthType` 以及 `Brand`/`Region`。
3. 若支持 live catalog，配置 `ModelsURL` 与 `ModelsAuth`；若有特殊鉴权 Header，配置 `AuthHeader` / `AuthScheme`。
4. 在 `internal/provider/registry.go` 的 `modelPrefixProviders` 中追加模型名称前缀，便于路由自动推断。

### 5.2 新增一个 CLI 工具适配器
1. 在 `internal/cli/` 下创建 `adapter_<name>.go`，实现 `cli.Adapter` 接口（`Status()`、`Apply()`、`Reset()`）。
2. 在 `internal/cli/registry.go` 中注册工具元数据（ID、名称、主页、支持的模型等）。
3. 在 `internal/cli/manager.go` 的 `Adapter(id)` switch 中引入新适配器实例。
