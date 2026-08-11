# Cyrene Gateway

Go 1.26+ 高性能 AI 代理网关，从 [9router](https://github.com/decolua/9router)（Next.js）重构而来。

提供 OpenAI 兼容的统一 API 端点（`/v1/*`），将流量智能路由到多个上游 AI Provider，支持多账号 fallback、模型别名、Combo 策略、OAuth 授权、配额追踪等。

## 特性

- OpenAI 兼容 API（`/v1/chat/completions`、`/v1/models`、`/v1/embeddings`、`/v1/messages`）
- 精选 Chat Provider 注册表（OpenAI、Anthropic、Gemini、DeepSeek、OpenRouter、Claude Code、Kimi Code、Qoder 等），批量 Provider 分批深度适配
- 多账号 fallback + 指数退避 + 连接状态管理
- 模型别名 & Combo 组合策略（fallback / round-robin）
- SSE 流式代理（断连感知）
- OAuth 授权流程（authorize / device-code / token import）
- 配额追踪 & Token Saver（RTK / Caveman / Ponytail）
- 代理池（出站代理轮换）
- Tailscale 入站隧道
- 内置管理面板（Vue 3 + Vite 8，玻璃拟态 UI，go:embed 单二进制分发）
- 第三方面板分发（`-panel-url dist.zip` 自动解压）
- SQLite 持久化（纯 Go，无 CGO）
- 结构化日志（slog）
- 多平台构建（Linux / Windows / macOS，Windows 含嵌入图标）

## 快速开始

### 下载

从 [GitHub Releases](https://github.com/arisvia/cyrene-gateway/releases) 下载对应平台的二进制文件。

### 运行

```bash
./cyrene-gateway -port 20128
```

浏览器打开 `http://localhost:20128` 即可进入管理面板。

### 使用

将你的 AI 工具（Claude Code、Cursor、Cline 等）的 API Base URL 指向：

```
http://localhost:20128/v1
```

### 添加 Provider

```bash
curl -X POST http://localhost:20128/api/providers \
  -H 'Content-Type: application/json' \
  -d '{"provider":"openai","name":"my-key","data":{"apiKey":"sk-..."}}'
```

### 设置模型别名

```bash
curl -X POST http://localhost:20128/api/models/alias \
  -H 'Content-Type: application/json' \
  -d '{"alias":"gpt4","target":"openai/gpt-4o"}'
```

## CLI 参数

```
-host 0.0.0.0       # 监听地址
-port 20128         # 端口
-dashboard ""       # 本地面板路径（开发用，空=使用内置）
-panel-url ""       # 远程面板 URL（dist.zip 自动解压，或单 HTML）
-secret ""          # Dashboard 访问密码
```

数据统一存放在 `~/.cyrene-gateway/`（数据库、面板缓存等），可通过 `CYRENE_DATA_DIR` 覆盖。

环境变量 `CYRENE_HOST` / `CYRENE_PORT` / `CYRENE_DATA_DIR` / `CYRENE_DASHBOARD` / `CYRENE_PANEL_URL` / `CYRENE_SECRET` 同样支持，flag 优先于 env。

## 开发

### 环境要求

- Go 1.26+
- Node.js 24 LTS（webui 构建，Vite 8 要求 >=22.12）

### 构建

```bash
# 构建面板
cd webui && npm ci && npm run build

# 构建二进制（面板自动嵌入）
go build ./cmd/gateway/
```

### 开发模式

```bash
# 前端热更新（:5173，proxy API 到 :20128）
cd webui && npm run dev

# 后端
go run ./cmd/gateway/ -port 20128
```

### 测试

```bash
go fmt ./...
go build ./...
go test ./...
```

## 项目结构

```
cmd/gateway/           # 入口 + Windows 资源（.syso 图标）
internal/config/       # CLI flags + env 配置
internal/db/           # SQLite 持久层
internal/model/        # 领域模型
internal/provider/     # Provider 注册表 & 模型解析
internal/handler/      # HTTP handlers（API + 面板服务）
internal/middleware/   # 中间件
internal/auth/         # 认证
internal/loopguard/    # 循环检测
internal/rtk/          # Token 压缩
internal/translator/   # 格式转换
internal/tunnel/       # Tailscale 隧道
webui/                 # Vue 3 + Vite 8 + TypeScript 面板工程
webui/embed.go         # go:embed all:dist
```

## 技术栈

- Go 1.26+（纯 Go，CGO_ENABLED=0）
- SQLite（modernc.org/sqlite）
- 标准库 net/http（Go 1.22+ 路由模式）
- Vue 3.5 + Vite 8.1 + TypeScript 5 + Pinia + vue-router
- lucide-vue-next（图标）
- 玻璃拟态设计系统（dark/light 双主题）

## Provider 支持矩阵

Phase 36 起采用**精选注册表**（curated registry）：只保留深度适配过传输层的 Chat Provider，
其余批量 Provider 将在后续分批完成深度适配；Media Provider（Embedding/TTS/STT/Image/Video/Web）走独立通道。

| 分组 | Provider | 说明 |
|------|----------|------|
| **精选编程工具** | Claude Code, OpenAI Codex, GitHub Copilot, Grok Build, Cursor, Qoder, Tencent Cloud CodeBuddy (intl/CN), Kimi Code, Google Antigravity, opencode, OpenRouter, GLM (intl/CN) | 已验证传输层正确性 |
| **官方直连 API** | Anthropic, OpenAI, Gemini, Vertex AI, Tencent Hunyuan, xAI, Alibaba Coding (intl/CN), Alibaba Studio, GitHub Models | 官方 API 端点 |
| **E2E 验证** | DeepSeek, Cerebras, Groq, NVIDIA, API.airforce | 端到端测试覆盖 |
| **品牌对/配额** | CodeBuddy (CN), Minimax (intl/CN) | 区域品牌分组 + 配额接口 |

### 传输格式

| 格式 | Provider 示例 |
|------|--------------|
| OpenAI (`/v1/chat/completions`) | OpenAI, DeepSeek, OpenRouter, Groq, Cerebras, NVIDIA |
| Anthropic (`/v1/messages`) | Anthropic, Minimax, GLM (claude endpoint) |
| Gemini (`:generateContent`) | Gemini |

### 免费 Provider 快速开始

```bash
# 一键启用所有免费 Provider（零配置，无需 API Key）
curl -X POST http://localhost:20128/api/providers/enable-free

# 立即使用
curl http://localhost:20128/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"opencode/claude-sonnet-4-20250514","messages":[{"role":"user","content":"hello"}]}'
```

## 进度

本项目正在从 9router 逐阶段重构，详见 `progress.json`。

## License

MIT
