# Changelog

所有显著变更记录于此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

- **API Key 细粒度控制与请求级 Context 穿透**：
  - **按 Key 传递 Context 与系统提示词注入**：在 `internal/auth` 中实现 Context 注入器（`WithAPIKey` / `APIKeyFromContext`），通过中间件管道向请求上下文贯通已认证的 API Key 实体；支持为 Key 配置预置 System Prompt（`systemPrompt`），并在响应缓存键生成（`cache.IsEligible`）前自动置入 OpenAI / Anthropic 消息体，天然分离不同业务端上下文的缓存哈希；
  - **模型访问白名单**：支持为单个 API Key 设定模型访问名单（`allowedModels`），支持通配符（如 `deepseek/*`、`*`）与精确匹配；在 `/v1/chat/completions`、`/v1/messages` 与 `/v1/embeddings` 入口在进入缓存查找前严格校验，未授权请求直接拦截并返回 `403 Forbidden`；`/v1/models` 列表接口按当前 Key 白名单动态过滤可见模型；
  - **独立速率限制 (RPM)**：API Key 支持配置专享限流上限（`rpm`），优先级高于系统全局 `settings.apiKeyRpm`；当全局未开启（0）但单个 Key 配置了 RPM 时精准生效；
  - **密钥生命周期与更新接口**：新增 `PUT /api/keys/{id}` 接口，支持在不重新生成 Secret 的情况下热更新名称、状态、白名单、RPM 与预设 Prompt；DB 迁移自动检测补齐 `apiKeys` 扩展字段；支持设置过期时间并在鉴权时拦截过期 Key；
  - **鉴权严格性增强**：即使用户未开启全局 `requireApiKey`，若客户端在请求头中携带了 API Key（Bearer / x-api-key），网关仍会严格校验其有效性与过期状态，非法或过期 Key 统一拦截（`401 Unauthorized`），合法 Key 则正常贯通请求 Context；
  - **WebUI 控制台升级**：首页 API Key 列表卡片直观展示模型权限、专享限流与 Context 注入状态胶囊；新增「规则」弹窗，提供完整的白名单、RPM 与 System Context 编辑能力。
- **统一能力标签体系与提供商市场重构**：
  - 后端 `/api/registry` 自动聚合与合成提供商的全量能力标签（`llm`、`image`、`tts`、`stt`、`video`、`embedding`、`web-search`、`web-fetch`），并将纯媒体提供商（如 ElevenLabs, Stability AI, Tavily, Exa）无缝合成进统一注册表市场；
  - 前端「提供商市场」作为所有凭证与连接创建的唯一入口，支持按能力标签与认证模式多维筛选、隐藏已接入项；
  - 「我的连接」专职承载 LLM 模型连接，提供商卡片固定名称与 ID 列宽，实现上下徽章队列规整排版；
  - 「多模态媒体工作台（Media）」全面升级为纯消费调试台，卡片仅在用户接入对应能力账号后点亮激活，未接入时引导一键前往市场。
- **Antigravity 深度协议适配与思考分级**：
  - 接入 Google Antigravity Code Assist 协议引擎与自动项目探测（`antigravity_discovery.go`），支持从凭证配置中穿透 Project ID；
  - 自动适配 Gemini 3.8 / 3.7 / 3.6 Flash 的思考深度分级（`high` / `medium` / `low`），向下游透传 OpenAI 标准 `reasoning_effort`；
  - 将 SSE `streamGenerateContent` 内联图像数据流实时聚合转换并回退为 OpenAI `/v1/images/generations` 标准响应；
  - 支持调用 Google Grounding 联网搜索能力，返回规整的搜索候选与引用锚点。
- **现代化矢量图标库改造**：
  - 新建 `webui/src/components/ui/icons.tsx` 集中矢量图标库，彻底清除非标准 Emoji 符号，统一采用极简描边风格与动态 `currentColor` 换肤；
  - 覆盖模型能力胶囊、操作按钮、交互状态（Check / Close / Lock / Edit / Settings）与空状态插画。
- **多模态与媒体服务精简收敛**：
  - 物理清理非主流媒体服务提供商（fal、recraft、cartesia、playht、assemblyai、runwayml 等），减少冗余适配与资源负担；
  - 彻底清除缺乏 WebSocket 支持的废弃 `edge-tts` 代码与图标。
- **数据目录灵活配置**：新增 `-data-dir` 命令行参数与 `CYRENE_DATA_DIR` 环境变量，允许指定数据库、密钥与面板缓存所在目录，实现测试与容器环境的彻底隔离。
- **提供商创建接口防线**：POST `/api/providers` 强化校验，强制要求 `provider` ID 必填（400）、`api-key` 类型密钥必填（400），并在同 provider+authType 已存在活跃连接时拦截重复创建（409）。
- **OAuth 回调 CSRF 防御**：GET `/api/oauth/{provider}/callback` 强制校验 `state` 必填且会话未过期，彻底杜绝无 state 绕过 PKCE 校验的安全风险。
- **CI / Release 职责解耦**：PR 与主干推送走 `build.yml` 门禁（类型检查、单元测试 `-race` 与单二进制冒烟测试）；Tag 推送专走 `release.yml`（多架构二进制交叉编译与 Docker 镜像推送）。
- **精准响应缓存（Exact Response Cache）与零延迟回放**：
  - 新增基于有界 LRU 与 TTL 的无锁快速响应缓存模块（`internal/cache`），支持毫秒级提取与零上游网络穿透；
  - 采用全量归一化请求体散列算法（`json.Marshal` 规范化 map，安全剔除 `stream` 传输标识），完整保留 `temperature`、`top_p`、`seed`、`tools`、`response_format` 等所有模型控制参数，彻底消除字段遗漏引发的哈希碰撞与误命中；
  - 在 `/v1/chat/completions`、`/v1/messages` 与 `/v1/embeddings` 三大核心入口处接入流式无侵入式响应捕获器（`ResponseRecorder`），仅对非流式 2xx 成功 JSON 响应建立缓存，安全绕过 SSE 事件流与错误状态；
  - 原生支持 `Cache-Control: no-cache`、`X-Cache-Control` 及 `Pragma` 客户端请求头穿透绕过；
  - WebUI「设置」页面整合缓存开关、TTL 配置、全局/确定性切换、实时条目与 Token 节省指标看板，以及一键清空管理。
- **RTK 令牌节省与多格式压缩强化**：
  - RTK 工具结果压缩引入无损预压缩流水线（ANSI 转义序列清洗、多余空行折叠、结构化 JSON `json.Compact` 就地紧凑化），大幅降低 Token 并使落入阈值内的工具数据完整无损保留；
  - 原生支持 Gemini `contents` 与 Anthropic 内容块结构，新增大于 16KB 单行/少行大文本的字符尺度截断兜底；
  - 完善 System Prompt 注入幂等性防御（覆盖 `messages`、`instructions`、`system` 及 `systemInstruction`），杜绝请求重试时提示词重复叠加；
  - Anthropic 协议转换保留 `cache_control` 断点标记（覆盖 tools、messages 文本块与工具结果），并在出站头中声明 `prompt-caching-2024-07-31`，支持长上下文与工具 Prompt Cache 命中；
  - 修复 Caveman 与 Ponytail 在未显式选级时因空字符串导致功能空转的缺陷，增加自动降级默认级别并在 WebUI 提供可视化级别配置；
  - 修复 `TokenSaverExclude` 排除规则对解析后提供商名字段的精确匹配。
- **架构瘦身与冗余边缘模块清理**：
  - 彻底移除与服务端核心职责脱节的 CLI 工具配置注入适配器（`internal/cli`）及对应前端页面与路由；
  - 彻底移除架构存在缺陷的 MITM 本地抓包代理（`internal/mitm`）：上游透传按 hostname 拨号在 hosts 劫持后导致自旋回环死锁、`X-MITM-Tool` 请求头全仓库无消费方、4 款目标工具中仅 Antigravity 勉强命中 URL pattern 实际兑现率仅 1/4；
  - 彻底移除越界执行系统特权安装脚本的 Tailscale 隧道管理（`internal/tunnel`）；
  - 将嵌入的 8 份技能描述平移为静态文档参考（`docs/skills/`），清理后端无业务逻辑的 `internal/skills` 模块与遗弃的未路由控制台（`Console.tsx`）；
  - 净削减 3,500+ 行非核心代码，消除了改写 hosts、特权安装脚本、盲写用户目录等安全隐患，将出站代理池（ProxyPools）正向纳入管理控制台核心导航；
  - **破坏性变更（Breaking Change）**：命令行参数 `-mitm` 与 `-mitm-port` 已移除，旧启动脚本若继续传递这些参数将在 `flag.Parse()` 时报错并退出进程。
### Fixed
- **出站请求全链路 SSRF 防护加固**：
  - 修复 Anthropic passthrough、Embeddings passthrough、后台模型同步与外部面板下载中绕过 `SafeHTTPClient` 的直接客户端构造，统一实施 dial-time IP 拦截与私网跳转防护；
  - `POST /api/providers/test-credentials` 与媒体凭证测试中前置 `ValidateUpstreamURL` 校验，拦截针对私网与云元数据地址（169.254.0.0/16）的探测请求；
  - 移除 `events.go` 中针对 `s.Events` 的冗余懒加载初始化，消除潜在并发竞态。
- **Tailwind CSS v4 样式与主题 Token 修复**：
  - 在 `@theme inline` 中补齐 `--color-foreground` 与 `--color-code-bg` 映射，激活全局 14 处页面与组件中因缺失 token 未生成 CSS 的 100 处 `text-foreground` 样式；
  - 修复 `Quota.tsx` 中 `onMount` 返回清理函数被 Solid 丢弃导致的 60 秒轮询定时器内存泄漏，重写为响应式 `createEffect` + `onCleanup`，并增加分页越界自动钳位（`effectivePage`）；
  - 全面清理前端应用与测试代码中的 `any` 与 `as any`，将 `api.ts` 的默认泛型收紧为 `unknown`，确保 100% 静态类型安全。
- **提供商与工具显示名精简**：统一将 `Google Antigravity` 精炼为 `Antigravity`，消除卡片名称列与能力徽章排版挤压；规范化 MiniMax 品牌大小写（`MiniMax (China)` / `MiniMax Coding`）；精简 CLI 适配器名称（`OpenAI Codex` 去除冗余 `CLI` 后缀）。
- **提供商市场双重滚动消除**：修复「提供商市场」标签页硬编码内层滚动容器（`max-h-[calc(100vh-220px)] overflow-y-auto`）导致的高度计算失准与外层双重滚动问题，解绑局部滚动约束，对齐全站统一的全局页面自然滚动与头部 `sticky top-16` 毛玻璃吸顶规范。
- **全站吸顶与页面间距规范统一**：将 `ProviderDetail` 头部吸顶栏与 `Media` 工作台头部统一对齐为全站标准的 `sticky top-16 z-20 bg-bg/90 backdrop-blur-md pt-1 pb-3 border-b border-subtle/50` 毛玻璃吸顶规范；将所有页面根容器纵向间距与入场动效统一为 `space-y-5 stagger`；为单行吸顶工具栏（`Console`、`Mitm`、`ProxyPools`、`Settings`、`Tunnel`）补全 `gap-3` 响应式防挤压间距。
- **连接卡片与多模态标签对齐**：固定卡片提供商名称与标识的容器宽度，解决名称长短不一导致的状态与能力徽章错位问题。
- **Antigravity 搜索模型探测**：修正联网搜索模型列表缺失 Gemini 3.8 / 3.6 的问题，精准适配 `gemini-3.8-flash-thinking` 与 `gemini-3.6-flash`。
- **媒体凭证验证与连接隔离**：`/api/media-providers` 接口支持 `connected=true` 服务端过滤，无连接媒体提供商不再污染工作台。
- **前端提供商连接类型错乱**：修复添加提供商表单未显式传递 `authType` 导致 OAuth/免费提供商被错误创建为 `api-key` 类型的逻辑缺陷；修复类型过滤与状态徽章枚举混用问题。
- **前端状态刷新机制优化**：`addProvider` 由本地数组拼接改为调用 `loadProvidersOnly` 服务端权威拉取，确保后端生成的连接 ID 与元数据完整展现。
- **构建产物断层治理**：从 Git 索引中解绑临时构建产物（`webui/dist/`），由 CI 在 Go 编译前重新构建最新 WebUI 产物嵌入，消除本地与历史提交中散落的幽灵哈希资产。
- **CSS 注释解析警告**：重写 `app.css` 中含斜杠星号的注释文本，消除 Lightning CSS 的 `Unexpected token Delim('*')` 构建警告。

### Added
- **MIT LICENSE**：项目正式以 MIT 许可证开源。
- **Prometheus 指标端点** `GET /metrics`（公开可抓取）：
  - `cyrene_requests_total{provider,model,endpoint,status}` 请求计数
  - `cyrene_request_duration_seconds{provider,endpoint}` 上游延迟直方图
  - `cyrene_tokens_total{provider,type}` token 计量（prompt/completion/cached/reasoning）
  - `cyrene_credentials_in_cooldown{provider}` 冷却中凭证数
  - `cyrene_build_info{version}` 构建版本
- **入站限流**：新增 `settings.apiKeyRpm`（0 = 关闭，默认关闭），对 `/v1/*` 按 API Key 每分钟固定窗口限流，超限返回 429 + `Retry-After`；设置即改即生效，无需重启。
- **Docker 支持**：多阶段 `Dockerfile`（面板构建 → Go 静态编译 → Alpine 运行镜像，非 root，含 HEALTHCHECK）、`docker-compose.yml`，CI 在 `v*` tag 时构建多架构镜像（amd64/arm64）并推送 ghcr.io。

### Fixed
- **面板白屏（P0）**：干净克隆构建出的二进制，管理面板因 `webui/dist/assets` 未纳入版本控制而 100% 白屏（JS 请求被 SPA 回退吞掉，返回 200 + text/html）。现构建产物已跟踪，且 `/assets/*` 缺失时正确返回 404（磁盘模式与嵌入模式双路修复），新增回归测试 `TestDashboardAssetMissIs404NotSPA`。
- **前端构建失败（P0）**：`vue-tsc 3.3.11` 与 `typescript 7`（Go 重写版）不兼容，`npm run build` 必然失败（CI 同样红）。`typescript` 钉回 `~5.9.3`。
- `.gitignore` 全局 `dist/` 规则误伤 `webui/dist/`（锚定为 `/dist/`）；`webui/.gitignore` 移除对 `dist/assets|providers|i18n` 的忽略。

## [历史版本]

未打 tag 前的开发史见 [git log](https://github.com/arisvia/cyrene-gateway/commits/main)（自 2026-07-21 起）。
