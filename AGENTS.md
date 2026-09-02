# AGENTS.md

> AI 编码智能体在此仓库协同工作时的参考规约。use context7

## 项目总览

**Cyrene Gateway** 是一个用 Go 1.26+ 编写的高性能 LLM API 统一网关，内置 Solid.js 单页管理控制台。主要代码分布在：
- `cmd/gateway/main.go`：服务启动入口、配置装配与优雅停机
- `internal/handler/`：OpenAI/Anthropic 兼容接口与管理端 RESTful API
- `internal/provider/`：各模型上游协议适配、凭证轮换、OAuth 流程、SSRF 防护与代理池
- `internal/db/`：基于 `modernc.org/sqlite` 的单连接 WAL 数据库存储
- `internal/config/`：命令行参数与环境变量解析（支持 `-data-dir` 自定义数据目录）
- `webui/`：基于 Solid.js + Vite + Tailwind CSS v4 的现代化内嵌管理控制台

## 常用命令

```sh
# 后端构建与全量测试
go build ./...
go vet ./...
go test -count=1 ./...

# 前端类型检查、测试与生产打包
cd webui
npm install    # 或 pnpm / bun / yarn
npm run typecheck
npm test
npm run build

```

## MCP 工具协同规约

本项目已接入四大 MCP 服务，智能体在处理不同领域任务时应优先调用对应工具：
1. **CodeGraph (`codegraph_explore`)**：代码架构拓扑与调用链图谱。在阅读/修改核心数据结构、函数调用链路、分析改动影响面（Blast Radius）时优先调用。
2. **Context7 (`context7/resolve-library-id`, `context7/query-docs`)**：实时三方库官方权威文档。在处理 Solid.js、Tailwind CSS v4、Vite、Go 新特性等第三方库时优先检索最新文档，杜绝过时语法。
3. **GitHub (`github/*`)**：代码仓库与协作流程。用于分支管理、PR 审查、Issue 跟踪、Secret Scanning 等。
4. **IntelliJ IDEA (`idea/execute_tool`)**：IDE 原生调试与编辑器同步。获取用户当前打开的文件、调用 IDE 原生重构、代码检查与运行配置。

## 代码规范与约束

1. **类型安全**：前端严禁使用 `any` 或 `as any`，使用 `unknown` 配合类型守卫（Type Guard）或标准领域接口。
2. **样式层纪律**：Tailwind CSS v4 基础元素重置与样式必须放入 `@layer base` 中，避免未分层样式击穿 `@layer utilities`。
3. **出站安全**：所有出站 HTTP 客户端必须通过 `SafeHTTPClient` 实施 SSRF 校验，严防私网与云元数据地址逃逸。
4. **提交规约**：遵循 `docs/git-commits.md`（Conventional Commits），保持提交粒度单一、原子化。

- 严禁在代码与测试用例中提交真实的 API Key、OAuth Client Secret 等敏感凭证。
- 未经明确指示，不得破坏 CI 门禁（`build.yml`）与多架构发布流程（`release.yml`）。
