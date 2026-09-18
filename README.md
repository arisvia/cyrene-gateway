# Cyrene Gateway

Cyrene Gateway 是一个自托管的 **LLM API 网关**:把众多 AI 提供商(OpenAI / Anthropic / Gemini / GitHub Copilot / Qoder 等数十家)统一收敛到一套 OpenAI 兼容 API 之后,并为它们补上连接池、故障回退(fallback)、配额冷却、用量统计与一块内置管理面板。

单二进制、零外部依赖(SQLite 内嵌)、Go + Solid.js 实现。

## 核心特性

- **统一 API 协议网关**：全面兼容 OpenAI 标准协议（`/v1/chat/completions`、`/v1/embeddings`、`/v1/images/generations`、`/v1/audio/*`、`/v1/models`、`/v1/responses`）及 Anthropic 原生协议（`/v1/messages`）。
- **全能力提供商市场**：聚合主流 LLM（OpenAI、Anthropic、Gemini、Qoder、OpenCode 等）与纯媒体/搜索服务（ElevenLabs、Stability AI、Tavily、Exa）；支持 API Key、OAuth（PKCE/设备码/Token 导入）、免密公共端点与 Cookie 认证。
- **智能调度与故障容灾**：支持多模型 Combo 轮转（Fallback、Round-Robin、Sticky 策略）；凭证池按优先级、模型锁与配额自动挑选；内置指数退避冷却（2s～5m）。
- **思考推理自适应映射**：下游标准化透传 `reasoning_effort` 与 `thinking` 参数，自适应转译至各主流大模型（Gemini 3.8/3.7、Claude 3.7、Qwen 等）。
- **生产级出站安全 (SSRF)**：所有出站流量经 `SafeHTTPClient` 实施拨号期 IP 严格过滤，防范私网探测、云元数据逃逸及 DNS 重绑定；支持轮询调度出站 HTTP/SOCKS 代理池。
- **用量监控与会话防护**：逐请求 Token 记账（含 reasoning/cached 消耗）与成本估算；内置 Loopguard 智能打断死循环；提供实时 SSE 事件流看板。
- **现代化控制台**：内置基于 Solid.js + Tailwind CSS v4 构建的单页管理后台，提供零依赖内嵌、深色玻璃拟物界面与即时演练场（Playground）。
## 快速开始

### 要求

- Go 1.26+
- Node.js 20+(仅构建面板时需要)

### 运行

```bash
# 1. 构建前端面板(产物嵌入二进制)
cd webui && npm ci && npm run build && cd ..

# 2. 构建并运行网关
go build -o cyrene-gateway ./cmd/gateway
./cyrene-gateway
```

启动后:

- 管理面板:<http://127.0.0.1:20128>
- OpenAI 兼容端点:`http://127.0.0.1:20128/v1/chat/completions`
- 健康检查:`GET /api/health`

数据目录为 `~/.cyrene-gateway/`(SQLite 数据库 `data.sqlite`)。

### 命令行参数与环境变量

| Flag | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| `-host` | `CYRENE_HOST` | `0.0.0.0` | 绑定地址(默认监听全部网卡以支持容器) |
| `-port` | `CYRENE_PORT` | `20128` | 网关端口 |
| `-data-dir` | `CYRENE_DATA_DIR` | `~/.cyrene-gateway` | 数据目录(数据库、密钥与面板缓存) |
| `-secret` | `CYRENE_SECRET` | 空 | 面板访问密码;空则自动生成并持久化 |
| `-dashboard` | `CYRENE_DASHBOARD` | 空 | 本地面板目录(开发用),空则用嵌入式面板 |
| `-panel-url` | `CYRENE_PANEL_URL` | 空 | 面板更新包 URL(dist.zip 或单 HTML),空则用嵌入式 |
| `-allow-private-networks` | `CYRENE_ALLOW_PRIVATE_NETWORKS` | `false` | 允许出站访问私网地址(本地 mock 测试用) |
| `-v`, `-version` | - | `false` | 打印当前构建版本并退出 |
### 使用示例

```bash
curl http://127.0.0.1:20128/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5",
    "messages": [{"role": "user", "content": "hello"}],
    "stream": false
  }'
```

在面板(或 `/api/providers`)添加提供商连接后,`model` 字段支持:

- 具体模型名(如 `claude-sonnet-4-5`)→ 按模型前缀推断提供商;
- Combo 名 → 按 combo 策略在多个模型间回退/轮转;
- 模型别名(面板中配置)。

## 开发

```bash
# 后端
go build ./...
go vet ./...
go test ./...          # CI 另会跑 go test -race ./...

# 前端
cd webui
pnpm install
pnpm typecheck        # tsc --noEmit
pnpm test             # vitest
pnpm build            # vite 构建生产产物
pnpm dev              # dev server :5173，自动代理 /api 到 :20128

### 目录结构

```
cmd/gateway/        # 入口:配置、数据库、HTTP server、优雅停机
internal/
  handler/          # HTTP 处理器(chat/embeddings/messages、管理 API、面板)
  middleware/       # Recovery / Logging / 请求体限制 / CORS / API Key / 面板会话
  auth/             # HMAC 密钥管理、API Key 签发、Argon2id 密码、登录限速
  provider/         # 提供商注册表、OAuth 流程、凭证调度、fallback、SSRF、代理池
  model/            # 连接/Combo 等领域模型、模型目录、live 模型发现与缓存
  db/               # SQLite 存储层(连接、组合、用量、KV、设置)
  translator/       # OpenAI ↔ Anthropic ↔ Gemini 响应格式互转
  usage/            # 各格式响应的 token 用量提取
  media/            # embeddings / TTS / STT / image / video / web 媒体端点
  loopguard/        # 对话死循环检测
webui/              # Solid.js + Vite 管理面板（构建产物由 CI 嵌入二进制，源码不提交 dist）
schema.sql          # 数据库 schema 参考(实际迁移在 internal/db/db.go)
```

### 发布

打 `v*` tag 触发 GitHub Actions,多平台交叉编译(linux/darwin/windows × amd64/arm64)并附到 Release。版本号通过 `-ldflags -X .../internal/handler.version=…` 注入,未注入时从 git build info 读取,回退 `dev`。

## 安全机制

- **外网智能识别与登录保护自适应**：智能探测客户端 IP 与 Host 头，非 localhost / 局域网访问时默认强制开启面板登录防护，自动阻断未授权远程接管；支持 `-admin-password` 参数或首启安全随机密码。
- **入站 DNS 重绑定与跨域防护**：管理接口（`/api/*`）严格实施可信回环校验，绑定验证 `Host` 与 `Origin`，阻断外部域名重绑定攻击与未授权跨域脚本。
- **CORS 严格分级**：推理端点（`/v1/*`）及公开探活开放跨域；管理接口仅对受信任本地 Origin 开放。
- **双重 SSRF 防御**：所有出站 HTTP 客户端统一经 `SafeHTTPClient` 托管，在拨号期拦截私网、回环、链路本地与云元数据地址（如 `169.254.169.254`），严防跳转逃逸。
- **访问控制与防爆破**：支持细粒度 API Key 白名单、专属 RPM 限流及 System Context 注入；面板密码采用 Argon2id 加盐哈希，并具备登录失败指数级 IP 锁定机制。
## License

以 [MIT](LICENSE) 许可证开源。
