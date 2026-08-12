# Cyrene Gateway 深度审计记录

- 审计日期：2026-08-12
- 审计范围：Go 后端核心请求链路、认证与管理 API、Provider 路由、SSE、远程面板、SQLite/凭据持久化、CLI 配置写入
- 审计方式：静态源码审查与调用链追踪
- 动态验证状态：当前审计环境未安装 Go，因此尚未执行 `go test`、`go test -race`、`go vet` 和二进制 E2E。本文件中的问题需在 Phase 37 实施时通过测试复核。

## 结论

当前实现已经具备清晰的 Go 工程分层和较完整的网关能力，但在默认安全边界、凭据返回、并发安全、请求大小控制和流式代理语义方面存在发布阻断项。Phase 37 完成前，不建议将服务以默认配置暴露到非可信网络。

建议按 P0 → P1 → P2 顺序修复，并保持每个修复点都有回归测试及 `-race` 验证。

## 核心调用链

```text
cmd/gateway/main.go
  -> config.Load
  -> auth.SetSecret
  -> db.Open / migrate
  -> handler.NewServer
  -> middleware: Recovery -> Logging -> CORS -> APIKeyAuth -> DashboardAuth
  -> /v1/chat/completions
  -> model / combo resolution
  -> credential selection + optional OAuth refresh
  -> transport resolution + auth injection
  -> upstream request
  -> error classification / fallback
  -> response translation / SSE forwarding
  -> usage persistence
```

## P0 发布阻断项

### P0-1 默认启动暴露未认证的管理 API 和 Provider 凭据

**证据**

- 默认监听地址为 `0.0.0.0`。
- `DefaultSettings()` 将 `RequireLogin` 和 `RequireAPIKey` 均设为 `false`。
- `DashboardAuth` 在 `RequireLogin=false` 时放行全部 `/api/*` 管理接口。
- `GET /api/providers`、创建和更新 Provider 的响应直接序列化 `ProviderConnection`。
- `ConnectionData` 包含 `apiKey`、`accessToken`、`refreshToken` 和 `providerSpecificData`。

**影响**

默认配置下，同一网络中可访问端口的客户端可能读取上游供应商密钥、OAuth token、代理配置，并调用管理、Tunnel、MITM 和 CLI 配置接口。这是完整凭据泄露与主机侧配置修改风险。

**整改**

1. 生产安全默认值必须二选一：默认仅监听 `127.0.0.1`，或首次启动强制初始化管理员认证。
2. 管理 API DTO 必须与持久化模型分离，响应中永不返回完整 secret。
3. 对已有 secret 仅返回 `hasApiKey`、`hasAccessToken`、掩码后四位等元信息。
4. Provider 更新采用字段级 patch，缺省 secret 表示保留，而不是用空结构覆盖。
5. Tunnel、MITM、CLI 写配置等高风险接口应实施独立权限检查，不能只依赖全局可关闭的登录开关。
6. 增加“默认配置下管理 API 不可被远程匿名访问”和“响应不含 secret”的回归测试。

### P0-2 登录限流 map 存在并发读写

`internal/auth/auth.go` 的包级 `attempts map[string]*lockEntry` 在 HTTP 并发处理路径中读、写、删除，但没有 mutex。可能触发数据竞争、计数丢失或 `concurrent map writes`。

**整改**

- 使用 `sync.Mutex` 保护一次完整的读取、过期判断和更新事务，或封装为独立 limiter 类型。
- 增加并发登录测试并执行 `go test -race ./...`。
- 增加容量上限和定期清理，避免攻击者使用大量伪造来源地址造成内存增长。

### P0-3 密码方案不适合作为交互式密码存储

Dashboard 密码使用服务全局 secret 进行 HMAC-SHA256。HMAC 可保证一致性，但不是慢密码派生函数。代码还保留默认密码 `123456`，最小长度仅 6。

**整改**

- 使用标准密码哈希方案，例如 Argon2id 或 bcrypt，并存储独立 salt 和参数。
- 删除可登录的默认弱密码。首次启用登录时要求显式设置强密码。
- 旧 HMAC hash 采用登录成功后迁移，不做静默破坏性升级。
- Session secret 与密码 hash salt 分离。

## P1 高优先级问题

### P1-1 请求和响应体缺少统一大小限制

聊天、Messages、多个 Media handler、OAuth/Qoder 错误响应存在不受控的 `io.ReadAll`。非流式上游响应也会完整载入内存。攻击者或异常上游可造成内存压力。

**整改**

- 在路由入口使用 `http.MaxBytesReader`，按端点设置 JSON、multipart 和媒体上限。
- 对上游错误体设置较小上限，例如 1 MiB，并标记截断。
- 对非流式模型响应设置可配置上限。
- `ParseMultipartForm(32 << 20)` 不能替代请求总大小限制。

### P1-2 自定义 BaseURL 和 panel-url 缺少 SSRF 策略

Provider connection 允许用户指定 BaseURL，服务端随后主动访问；`-panel-url` 也由服务主动下载并跟随默认重定向。若管理面被攻破或配置来源不可信，可访问回环、链路本地、内网和云元数据地址。

**整改**

- URL 仅允许 `https`，开发模式可显式允许 loopback HTTP。
- DNS 解析后拒绝 loopback、link-local、multicast、unspecified、私网地址，或提供明确的 `allow-private-upstream` 开关。
- 对每次重定向重新校验目标。
- `panel-url` 建议增加内容签名或固定 SHA-256 校验；至少禁止敏感网段和非 HTTP(S) scheme。

### P1-3 SSE 解析器破坏通用 SSE 语义

`proxyStreaming` 按行扫描并跳过空行，再自行输出 `data: ...\n\n`。这会丢失多行 data 事件的聚合语义、注释心跳、部分 `event:`/`id:` 与 data 的事件边界。Scanner 的 1 MiB 单行限制触发后，代码没有检查 `scanner.Err()`，仍发送 `[DONE]`，会把截断伪装成成功结束。

**整改**

- 实现事件级 SSE parser，按空行提交完整事件，保留多行 data、event、id、retry 和注释。
- 明确区分正常 EOF、上游取消和扫描错误。
- 扫描错误后不要伪造 `[DONE]`，应记录失败并终止连接。
- 为多行事件、超长 chunk、心跳、半包、客户端断连增加测试。

### P1-4 流式连接受固定 5 分钟超时限制

HTTP server `WriteTimeout=5m`，上游 `http.Client.Timeout=5m`。长时间 agent/tool-call 会话可能被网关主动切断。`Client.Timeout` 覆盖读取完整响应体，不适合不定长 SSE。

**整改**

- 流式和非流式使用不同 client 策略。
- 流式请求取消总时长 timeout，改用连接、TLS、响应头和空闲读取超时。
- 服务端使用 `ResponseController.SetWriteDeadline` 做按写入续期的 idle timeout，避免固定总时长。

### P1-5 Combo 路径与单模型路径行为不一致

Combo 路径把响应固定按 `translator.FormatOpenAI` 处理，即使上游 transport 是 Anthropic 或 Gemini；它没有完整复用单模型路径中的请求翻译、loop guard、termination prompt、token saver、max token clamp、成功状态重置和错误状态持久化。当前 `CheckFallbackError` 对所有未知错误默认 fallback，因此注释中的 non-fallback 分支实际上不可达。

**整改**

- 抽取统一的 `executeAttempt` 管线，单模型和 combo 只负责提供候选列表。
- 管线统一处理 transport、translation、refresh、状态更新、usage 和 response format。
- 定义明确的 fallback 状态集。通常 400/422 等确定性请求错误不应轮询全部账号或模型。
- 添加 Anthropic/Gemini combo、400 不 fallback、429 fallback、OAuth refresh 的 E2E 测试。

### P1-6 Secret 文件位置不服从 DataDir

`auth` 包在 `init()` 时从 `~/.cyrene-gateway/auth-secret` 加载密钥，而数据库和其他状态可固定在当前用户数据目录。这会导致容器备份、实例隔离和迁移时 session/API key 签名失效或串用。

**整改**

- 移除认证包的全局 `init()`，在 `main` 读取 config 后显式创建 secret manager。
- secret 必须位于固定的应用数据目录，并检查目录、文件权限及读写错误。
- 多实例部署时要求显式注入稳定 secret。

### P1-7 Secret 和配置文件写入不是原子更新，权限过宽

CLI 通用写入使用 `0644`，其中包含 `auth.json`、`secrets.json` 和 `.env`。写入直接覆盖目标文件，进程崩溃时可能留下截断配置；部分 Reset 忽略写入/删除错误。

**整改**

- 敏感文件使用 `0600`，目录使用 `0700`。
- 写入临时文件、fsync、rename，并尽量保留原文件权限。
- 所有安全相关写入必须传播错误。
- 对外部工具配置执行备份和可恢复更新。

## P2 中优先级问题

### P2-1 CORS 策略过宽且字段不完整

全局返回 `Access-Control-Allow-Origin: *`，同时管理 API 可能无认证；允许头未包含实际支持的 `x-api-key`。建议将 Dashboard 管理 API 与 `/v1/*` 的 CORS 分开配置，默认仅允许 same-origin，并显式配置可信 origin。

### P2-2 OAuth redirect_uri 接受任意输入

Authorize 接口接受查询参数 `redirect_uri` 并保存进 session。state/PKCE 已实现，但仍建议对每个 Provider 使用固定回调地址或 allowlist，避免 OAuth 配置误用和 token 流程被引向非预期地址。

### P2-3 数据库更新存在读改写覆盖风险

连接状态、OAuth refresh、测试状态和自定义模型会读取整个 `ConnectionData` 后整体更新。并发请求可能互相覆盖 token、cooldown、quota 或 providerSpecificData。SQLite 单连接只串行执行 SQL，不能保证跨多条语句的业务原子性。

**整改**

- 为状态字段提供窄更新 SQL，或使用事务与版本号做 optimistic locking。
- token refresh 结果、error state、test state 分开更新。
- 对同一 connection 的内存修改避免共享可变 map。

### P2-4 错误处理和可观测性不完整

多个 `rand.Read`、`DB.UpdateConnection`、`json.Marshal/Unmarshal` 和文件操作忽略错误。部分写客户端响应后也不检查错误。应建立“允许忽略错误”的明确白名单，其余通过日志或返回值处理。

### P2-5 HTTP server 缺少 ReadHeaderTimeout 等边界

当前设置 ReadTimeout、WriteTimeout 和 IdleTimeout，但没有单独的 `ReadHeaderTimeout`、`MaxHeaderBytes`。建议为公开部署提供保守默认值，并区分流式端点。

### P2-6 远程面板解压限制存在边界偏差

`io.LimitReader(rc, maxPanelFileSize)` 只会截断超过限制的文件，并不会检测“实际超限”；下载限制也没有通过读取额外 1 byte 判断响应是否超过 20 MiB。面板可能被静默截断后写入缓存。

**整改**

- 读取 `limit+1` 并在超限时失败。
- 校验 zip entry 的 `UncompressedSize64`，同时保留流式硬限制。
- 临时目录解压完成且验证通过后再原子替换正式目录。

## 设计级优化建议

1. 将持久化模型、内部领域模型和 API DTO 分开，特别是包含 secret 的类型。
2. 抽取统一 Upstream Executor，消除 chat/combo/messages/test-model 的 transport 与错误处理分叉。
3. 提供统一安全 HTTP client factory，集中配置代理、SSRF 校验、重定向、连接池和各类 timeout。
4. 引入端点级请求限制中间件，避免每个 handler 自行处理。
5. 将全局可变状态改成 Server 依赖，包括认证 secret、登录 limiter、OAuth session store 和缓存。
6. 为生产构建增加安全基线：`go vet`、`go test -race`、govulncheck、前端测试、secret response contract tests。

## Phase 37 完成标准

- [ ] P0 全部关闭，并有回归测试
- [ ] 管理 API 响应中不出现完整 apiKey/accessToken/refreshToken/cookie
- [ ] 默认启动不对非可信网络提供匿名管理权限
- [ ] `go test ./...`、`go test -race ./...`、`go vet ./...` 通过
- [ ] 核心 E2E 覆盖 OpenAI、Anthropic、Gemini 的流式、非流式和 combo
- [ ] SSE 覆盖多行事件、心跳、超长事件、上游异常和客户端断连
- [ ] 请求体、响应体和 multipart 均有硬上限
- [ ] 自定义 URL 和 panel-url 有明确 SSRF 策略
- [ ] 文档给出本地模式与远程部署的安全配置示例

## 建议实施批次

### 37A 安全边界

修复默认监听与认证、secret DTO、登录 limiter、密码迁移、application secret lifecycle。

### 37B 请求执行管线

统一 single/combo/messages executor，修复 fallback 分类、状态更新和 transport translation。

### 37C 流式与资源限制

重写 SSE event parser，拆分 timeout，增加 body/header/response 限制。

### 37D 主动网络与文件安全

加入 SSRF/redirect policy、远程面板签名或 hash、原子安全写入、权限收紧。

### 37E 验证与发布门禁

执行 race/vet/vuln/E2E，补充 CI 和部署安全文档。
