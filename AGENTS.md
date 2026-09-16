# AGENTS.md

> AI 编码智能体在此仓库协同工作时的参考规约。

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
4. **提交规约**：遵循 Conventional Commits（`feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`），保持提交粒度单一、原子化。
5. **动态模型同步纪律**：所有提供商（Provider）严禁在 `internal/provider/registry_data.go` 中硬编码静态 `Models` 列表（除纯离线/专有无 catalog 协议端点外）。所有模型必须保持与上游动态拉取与缓存机制同步（统一配置 `ModelsURL` 或专属动态抓取器），杜绝由于本地写死模型列表导致与上游最新目录漂移。
6. **UI 组件一致性纪律**：
   - 页面与卡片空状态一律复用 `@/components/ui` 的 `<Empty />`（Rich 模式传 `icon`、`title`、`description`、`action`；紧凑模式传 `message`），严禁各业务页面私造空状态 DOM 或重复放置 CTA 按钮。
   - 严禁在业务组件中内联书写 Raw `<svg>` 路径，所有图标统一收口至 `@/components/ui` (`icons.tsx`) 导出。
   - 严禁散落使用原生 HTML 表单元素（如裸 `<input>`、裸 `<button>` 及自定义 spinner），必须使用统一的 `Input`、`Button`、`Select`、`Alert`。
7. **反重复造轮子与工具函数统一步调（Deduplication & Zero Redundant Wheels）**：
   - 严禁各模块散落手写 Base64URL 编解码、PKCE 签名生成、UUID 生成；统一收口至 `internal/handler/helpers.go` 或 `uuid.New().String()`。
   - 严禁各流式转发模块私造 SSE 数据行解析；统一使用 `internal/translator/translator.go` 的 `translator.ParseSSEDataLine(line)` 或 `ParseSSEDataLineString(line)`（兼容带空格与无空格规范）。
   - 前端严禁在组件中私自实现 `formatBytes`、`copyToClipboard` 等基础功能；统一收口至 `@/lib/format` 与 `@/lib/clipboard`（并保留 `document.execCommand` 降级容灾）。
   - 测试用例中严禁手写 contains 字符串或切片搜索辅助函数；一律优先使用 Go 标准库 `slices.Contains` 与 `strings.Contains`。
8. **全量国际化与文案纪律（Zero Hardcoded Strings & Strict i18n）**：
   - 前端所有页面、组件、模态框、通知与选项一律严禁在 JSX/TSX 中硬编码中/英文自然语言字符串。
   - 所有文案必须在 `webui/src/i18n/zh-CN.ts` 与 `webui/src/i18n/en-US.ts` 中双向严格对齐，通过 TypeScript `NestedKeyOf<typeof zhCN>` 静态类型门禁检查。
9. **极简架构与反代码堆砌（Anti-Bloat & Lazy Engineering）**：
   - 严禁编写“为了未来可能用到”的无引用抽象层、单实现接口、多余中间转换或空壳工厂模式（坚持 YAGNI 原则）。
   - 坚决优先使用平台原生与标准库方案（如 Go 标准库 `flag` 单/双横杠自动等价、SQLite WAL checkpoint TRUNCATE、CSS 原生毛玻璃），拒绝无评估引入非必要第三方库。
   - 保持重构干净切割（Clean Cutover）：废弃旧方案时必须同步移除全部过时注释、僵尸辅助函数与历史死分支。
10. **系统级端口交接与容器防呆（Process Handover & Container Safety）**：
    - 服务自重启与端口平滑交接时，必须先关闭旧 HTTP Listener 释放端口，再启动带 `CYRENE_RESTART=1` 的子进程进行重试绑定，杜绝 Windows/Linux 下的 `EADDRINUSE` 端口冲突。
    - 凡涉及物理二进制覆盖、原地重启或宿主系统级别操作，必须优先通过 `system.IsRunningInDocker()` 探测，容器环境下坚决阻断并给出拉取镜像提示，避免容器内 PID 1 退出导致的死循环崩溃。
11. **本地预提交与 CI 门禁一致性纪律（Strict Pre-Commit & CI Consistency）**：
    - 每次 git commit 之前，必须在本地完整执行与 GitHub Actions `build.yml` 严格一致的全套门禁命令：
      ```sh
      # 1. Go 代码格式化与静态检查（绝对零容忍格式差异）
      gofmt -w .
      if [ -n "$(gofmt -l .)" ]; then echo "Unformatted Go code found:"; gofmt -d .; exit 1; fi
      go vet ./...
      # 2. 前端类型检查、单元测试与生产打包
      cd webui && npm run typecheck && npm test && npm run build && cd ..
      # 3. 后端全量测试、竞争检测与编译验证
      go test -count=1 ./...
      go test -race ./...
      go build ./...
      ```
    - 严禁未经本地全套检验盲目提交，杜绝把未格式化（`gofmt`）或破坏静态检查的代码推入仓库。
12. **滚动容器防裁切与焦点环纪律（Scroll Container & Focus Ring Discipline）**：
    - 凡 `overflow-y-auto` 等产生 CSS 滚动条的容器，严禁单侧只加 `pr-*`，必须使用对称的 `px-*`（如 `px-1`、`px-1.5`），为子元素向外扩散的 `focus:ring`（`box-shadow`）预留呼吸缓冲区，杜绝左边缘被裁剪。
    - 凡包裹交互控件的转场过渡容器（如 `TabTransition`），严禁全时段硬编码 `overflow-x-hidden`，必须仅在活跃动画期间按需激活（`isTransitioning() ? 'overflow-x-hidden' : ''`），静态状态下保持自然的 `visible` 溢出。
    - 表单项组件（如带步进器的输入框）本身已常驻展示当前数值时，上层标题标签行严禁重复显示相同的动态数值，保持单一数据源。
13. **文档极简与单一事实源（Lean Documentation & Zero Noise）**：
    - 严禁将一次性临时会话流水账、过期草稿、历史迁移蓝图或废弃接口文档遗留在代码库中。
    - 维护精简统一的权威事实源：架构看 `docs/ARCHITECTURE.md`，规约看 `AGENTS.md`，版本看 `CHANGELOG.md`，对外能力看 `docs/skills/`，杜绝相互冲突的多重文档增加 AI Agent 的上下文消耗与幻觉误导。

- 严禁在代码与测试用例中提交真实的 API Key、OAuth Client Secret 等敏感凭证。
- 未经明确指示，不得破坏 CI 门禁（`build.yml`）与多架构发布流程（`release.yml`）。
