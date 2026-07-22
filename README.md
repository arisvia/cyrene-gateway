# Cyrene Gateway

Go 1.26+ 高性能 AI 代理网关，从 [9router](https://github.com/decolua/9router)（Next.js）重构而来。

提供 OpenAI 兼容的统一 API 端点（`/v1/*`），将流量智能路由到多个上游 AI Provider，支持多账号 fallback、模型别名、Combo 策略等。

## 特性

- OpenAI 兼容 API（`/v1/chat/completions`、`/v1/models`、`/v1/embeddings`）
- 多 Provider 支持（OpenAI、Anthropic、Gemini、DeepSeek、OpenRouter 等 15+）
- 多账号 fallback + 指数退避
- 模型别名 & Combo 组合策略
- SSE 流式代理
- SQLite 持久化（纯 Go，无 CGO）
- 结构化日志（slog）
- 多平台构建（Linux / Windows / macOS）

## 快速开始

### 下载

从 [GitHub Releases](https://github.com/arisvia/cyrene-gateway/releases) 下载对应平台的二进制文件。

### 运行

```bash
./cyrene-gateway -port 20128 -db data.sqlite
```

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

## 开发

```bash
# 构建
go build ./cmd/gateway/

# 运行
go run ./cmd/gateway/ -port 20128 -db data.sqlite

# 格式化
go fmt ./...

# 测试
go test ./...
```

## 项目结构

```
cmd/gateway/           # 入口
internal/config/       # 配置
internal/db/           # 数据库层
internal/model/        # 领域模型
internal/provider/     # Provider 注册 & 模型解析
internal/handler/      # HTTP 处理
```

## 技术栈

- Go 1.26+（纯 Go，CGO_ENABLED=0）
- SQLite（modernc.org/sqlite）
- 标准库 net/http（Go 1.22+ 路由模式）

## 进度

本项目正在从 9router 逐阶段重构，详见 `progress.json`。

## License

MIT
