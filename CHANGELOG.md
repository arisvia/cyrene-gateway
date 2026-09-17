# Changelog

所有显著变更记录于此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [1.2.0] - 2026-09-17

### Added
- **OpenCode 提供商适配与双端点支持**：
  - 新增 `opencode`（OpenCode 官方端点）与 `opencode-go`（OpenCode Zen 端点），并在 WebUI 提供商市场与详情页统一呈现对应图标与操作向导。
  - 在 `internal/provider/registry_data.go` 注册 OpenCode 系列模型元数据与免密能力，支持上游动态模型拉取与自动同步。
  - 支持通过 `ProviderSpecificData["sessionId"]` 注入自定义会话 ID，保证跨请求上下文黏性。
- **系统管理与在线运维能力（Ops）**：
  - 系统详情指标监控：实时呈现系统 CPU、内存、Goroutine、数据库大小及 WAL 状态。
  - SQLite 运维维护：支持在线 WAL Checkpoint（`TRUNCATE`）与 `VACUUM` 物理收缩整理。
  - 跨平台二进制原地热升级：官方 GitHub Release SHA-256 完整性校验、原子替换与自动 `.old` 备份（容器环境下自动探测阻断并提示镜像拉取）。
  - 无缝自重启与端口平滑交接：基于 `CYRENE_RESTART=1` 的子进程接管与重试绑定，彻底解决 Windows/Linux 端口占用冲突。

### Fixed
- **OpenCode 免费档客户端指纹防 403 阻断**：
  - 严密对齐 OpenCode 官方客户端指纹协议，将 `User-Agent` 升级为 `opencode/1.18.31`，消除裸 `opencode` 触发的 `403 FreeTierError`（上游限制版本 `>= 1.17.0`）。
  - 实现官方规范的 30 位降序会话 ID（`ses_` + 12 位 hex 时间戳 + 14 位 Base62 字符）与请求 ID（`msg_` + 12 位 hex 时间戳 + 14 位 Base62 字符）生成及规范化映射。
  - 修复 `test_model.go` 闪电快速测试未接入 `ApplyAuth` 与客户端指纹、导致 `big-pickle` 等免密模型测试失败的问题。
  - 修复 `test_batch.go` 探针未走免费档模型、导致未充值 Key 阻断向导保存的问题，统一收口为 `big-pickle` + `Bearer public` 探测。
- **WebUI 演练场（Playground）工作台与视觉重构**：
  - 重构演练场为固定视口工作台布局，彻底消除外层滚动抖动。
  - 消除会话历史模式切换时非活跃轮次的串扰与渲染混淆。
  - 移除 Max Tokens 输入框标题栏冗余数值展示，保证单一数据源。
- **CSS 滚动容器防裁切与焦点环优化**：
  - 为所有 `overflow-y-auto` 容器补齐对称的 `px-*` 呼吸缓冲区，杜绝子元素 `focus:ring` 阴影左边缘被截断。
  - 修复「用量看板」Token 趋势悬浮 Badge 动态挂载触发的标题栏高度抖动与微小布局偏移（CLS），锁定 `h-6` 容器并通过平滑透明度过渡替代条件装载。
  - 修复「演练场」空状态面板多层嵌套内缩与虚线外框导致的视觉偏小 Bug，拉齐垂直中轴与外层卡片边界。
  - 移除 Max Tokens 输入框标题栏冗余数值展示，保证单一数据源。
- **凭据解析与模型级路由统一收口**：
  - 在 `internal/provider/transport.go` 抽象 `ResolveCredentials(conn, providerName, modelName)` 统一门面函数，消除各 handler 散落的凭据构造与 OpenCode 免费模型 `Bearer public` 覆盖逻辑。
- **精简架构与规约沉淀**：
  - 在 `AGENTS.md` 固化严格的本地 CI 预提交一致性门禁（Rule 11）、滚动容器防裁切规范（Rule 12）与文档单一事实源纪律（Rule 13）。
  - 移除过期草稿与历史蓝图文件，防止智能体上下文冗余。

## [1.1.0] - 2026-09-16

### Security
- **OAuth 出站流 SSRF 防护增强**：统一 `oauth.go`、`oauth_flow.go`、`qoder.go`、`qoder_exec.go` 以及 `qoder_pat.go` 出站 HTTP 请求经 `SafeHTTPClient`（或 `Server.getHTTPClient()`）代理与校验，阻断针对内网与私网元数据地址的探测逃逸；在 `InitiateQoderDeviceFlow` 中补充空指针安全防护。

### Added
- **`-dashboard` 命令行参数与本地开发模式支持**：在 `config.Load` 中激活 `-dashboard`（及 `CYRENE_DASHBOARD` 环境变量），支持本地挂载未打包的前端静态目录进行热开发。
- **结构化 CLI 帮助文案与环境变量对照**：按 Server、Security、Dashboard、General 分组输出清晰的命令行参数指引，修正 `-secret` 的 HMAC Master Secret 语义说明，并标注对应的 `CYRENE_*` 环境变量。

### Refactored
- **核心算法与轮子去重**：
  - 统一全库 Base64URL 编解码于 `provider/oauth.go` 标准库实现，消除重复手写转换。
  - 统一 RFC 7636 PKCE 代码验证码与挑战码计算逻辑。
  - 提取标准 `translator.ParseSSEDataLine` 解析器，统一处理 `data:` 与 `data: ` 差异，收口所有 SSE 行解析。
  - 统一 UUID 随机 ID 生成逻辑至 `google/uuid`。
- **前端工具函数收口与图标规范化**：
  - 消除前端业务组件散落内联的 raw `<svg>` 路径，在 `@/components/ui/icons.tsx` 集中导出标准化 SVG 图标。
  - 统一字节格式化函数 `formatBytes` 至 `webui/src/lib/format.ts`。
  - 抽离通用跨平台剪贴板复制工具 `copyToClipboard`（支持现代 Clipboard API 与 `document.execCommand` 降级兜底）。

### Testing
- **配置契约测试**：重构 `config.Load` 委托至 `registerFlags`，引入 `TestFlagContract` 测试全量 9 个 CLI 参数在单横杠 `-` 与双横杠 `--` 混合输入下的解析契约。
- **清理重复字符串匹配测试辅助函数**：移除手写 `contains` 辅助逻辑，替换为标准库 `strings.Contains` 与 `slices.Contains`。

## [1.0.0] - 2026-09-15

### Security
- **DNS Rebinding 与跨域本地劫持防御（P0）**：在管理端中间件 `DashboardAuth` 引入可信回环校验（`Trusted Loopback`），强制联合校验客户端 `RemoteAddr`、HTTP `Host` 及 `Origin` 请求头；任何将域名指向 127.0.0.1 的外部重绑定请求或第三方网页跨域请求将被直接拒发回环信任，强制实施 Session 登录校验（401 Unauthorized）。
- **管理端 CORS 隔离防护**：管理端接口（`/api/*`）仅对本地可信回环 Origin 开放跨域；公有接口（`/v1/*`）及公开探活端点（`/api/health`）对外部应用保持全开放兼容。
- **全链路出站 SSRF 双重校验**：所有出站 HTTP 客户端统一经 `SafeHTTPClient` 托管，实施 Dial-time 解析期与拨号期 IP 校验，阻断针对内网、回环、链路本地与云元数据地址（如 169.254.169.254）的探测，并拦截 302 重定向逃逸。
- **凭证探测前置防线**：`POST /api/providers/test-credentials` 与媒体凭证测试中前置 `ValidateUpstreamURL` 校验，拦截私网探测。
- **OAuth 回调 CSRF 防御**：`GET /api/oauth/{provider}/callback` 强制校验 `state` 必填且会话未过期，彻底杜绝无 state 绕过 PKCE 校验的安全风险。
- **暴力破解防护**：登录失败按 IP 指数锁定（30s → 30m）；密码存储采用 Argon2id 单向加盐哈希（自动兼容旧版 HMAC 迁移）。

### Added
- **Claude Code 1M 上下文标记剥离**：自动剥离客户端传递的 `[1m]` / `[1M]` 上下文后缀（如 `claude-opus-5[1m]`），避免上游报 404 模型未找到（对标 9router#3690）。
- **Codex Tool Schema Unicode 正则清洗**：拦截并清洗 Tool Schema 中包含未转义 `\p{...}` 的正则约束，彻底解决 Codex 上游 WAF/校验器 400 Invalid Schema 报错（对标 9router#3922）。
- **Claude Code 官方兼容与私有协议剥离**：针对 Claude Code 2.1.270+ 会话初始化发送私有 `output_config.format` 导致第三方网关 400 熔断的问题，对非官方 Anthropic 节点实施 `.format` 自动剔除并保留 `effort` 思考等级；对官方 Anthropic/Claude 通道实施三重锁豁免保护。
- **并行工具结果连续聚合**：在 `openAIToClaude` 协议转换中加入前序消息类型嗅探，严格遵守 Anthropic 交替轮次协议，将连续并行的 `role: "tool"` 结果自动聚合成单条 `role: "user"` 消息。
- **Anthropic Cache Control 预算限制**：引入 `trimClaudeCacheControl`，严格限制单请求最多保留 4 个 `cache_control` 标记，自动丢弃多余 marker 防止 400 报错。
- **多模态与单对象 Content 兼容**：独立 `image_url` 解析分支，支持 Data URI Base64/外链转译；兼容 OpenAI 单字典对象 `content: {"type": "text", ...}`。
- **精确响应缓存（Exact Response Cache）与零延迟回放**：基于有界 LRU 与 TTL 的无锁快速响应缓存模块（`internal/cache`），采用全量归一化请求体散列算法，支持毫秒级提取与零上游网络穿透；支持客户端请求头穿透绕过。
- **API Key 细粒度控制与请求级 Context 穿透**：支持为 API Key 配置预置 System Prompt、模型访问白名单（通配符支持）、独立专属 RPM 限流；支持 `PUT /api/keys/{id}` 热更新。
- **Prometheus 指标端点**：公开可抓取端点 `GET /metrics`，暴露请求总数、上游延迟直方图、各类型 Token 计量、冷却中凭证数与构建版本。
- **入站限流与防死循环**：新增全局 `settings.apiKeyRpm` 速率限制；配置 `loopguard` 工具调用与复读死循环检测，防额度意外击穿。
- **CLI 版本输出**：支持 `-v` 与 `-version` 命令行参数打印版本并退出。
- **MIT 开源许可证**：项目正式以 MIT 许可证开源。
- **Docker 容器化支持**：多阶段 `Dockerfile`（Alpine 最小运行镜像，非 root，内置 HEALTHCHECK）与 `docker-compose.yml`。

### Changed
- **统一能力标签体系与提供商市场重构**：后端 `/api/registry` 自动聚合与合成提供商的全量能力标签（`llm`、`image`、`tts`、`stt`、`video`、`embedding`、`web-search`、`web-fetch`），并将纯媒体提供商（如 ElevenLabs, Stability AI, Tavily, Exa）无缝合成进统一注册表市场。
- **架构瘦身与冗余边缘模块清理**：物理移除与服务端核心职责脱节的 CLI 工具注入适配器（`internal/cli`）、MITM 抓包代理（`internal/mitm`）与 Tailscale 隧道管理（`internal/tunnel`），净削减 3,500+ 行非核心代码。
- **Antigravity 深度协议适配与思考分级**：接入 Google Antigravity Code Assist 协议引擎与自动项目探测，支持 Gemini 3.8 / 3.7 / 3.6 Flash 思考深度分级并向下游透传 OpenAI 标准 `reasoning_effort`。
- **现代化矢量图标库**：集中矢量图标库替代 Emoji 符号，统一采用极简描边风格与动态 `currentColor` 换肤。
- **数据目录灵活配置**：新增 `-data-dir` 命令行参数与 `CYRENE_DATA_DIR` 环境变量，实现运行与存储目录彻底隔离。

### Fixed
- **令牌节省引擎多执行器全链路闭环**：为 Google Antigravity 与 Qoder 等独立握手协议执行器补齐 `applyTokenSaver` 调度，彻底解决专有通道下 RTK 工具结果压缩与 Caveman/Ponytail 提示词静默失效的问题。
- **令牌节省排除名单大小写与模型前缀归一化**：提炼 `isTokenSaverExcluded` 统一匹配逻辑，支持大小写不敏感匹配以及模型厂商前缀匹配（如 `anthropic/*`），并在预翻译前工具压缩阶段保持统一。
- **WebUI 核心空状态组件升级与全站视觉归一化**：全面升级 `<Empty />` 组件支持微光悬浮图标与富操作布局，统一收口 Combos、ProxyPools、Providers、Media、Quota 5 处空状态，消除散落原生控件与内联 Raw SVG。
- **Gemini / Antigravity 多轮对话结构归一化**：自动合并相邻同角色消息，确保首轮为 `user` 角色并过滤空 parts，规避 400 INVALID_ARGUMENT 轮次报错（对标 9router@e7b5f09）。
- **Gemini Schema 兼容性增强**：实现 `prefixItems` 元组模式向 `items` 的自动平铺映射，为缺失 `items` 的 `type: "array"` 提供安全占位，杜绝 400 模式校验中断（对标 9router@f6c59d3）。
- **连接重验与状态重置闭环**：`ResetAccountState` 增加对 `modelLock_*` 细粒度模型锁的自动清理，保证连接重测或成功请求后彻底重置全部陈旧熔断标记（对标 9router#3810, #3830）。
- **面板白屏治理（P0）**：干净克隆构建出的二进制，管理面板因 `webui/dist/assets` 未纳入版本控制而白屏的问题彻底解决；`/assets/*` 缺失时正确返回 404，新增回归测试 `TestDashboardAssetMissIs404NotSPA`。
- **前端构建兼容性**：治理 `typescript 7` 兼容性，钉回 `~5.9.3`。
- **Tailwind CSS v4 样式与主题 Token 修复**：补齐 `--color-foreground` 与 `--color-code-bg` 映射，激活全局页面 `text-foreground` 样式。
- **定时器内存泄漏修复**：修复 `Quota.tsx` 中轮询定时器内存泄漏，重写为响应式 `createEffect` + `onCleanup`，增加分页越界自动钳位。
- **前端全量类型安全**：全面清理前端应用与测试代码中的 `any` 与 `as any`，默认泛型收紧为 `unknown`。
- **提供商与页面滚动治理**：修复「提供商市场」标签页双重滚动问题；全站吸顶与页面间距规范统一为毛玻璃吸顶规范。
- **媒体凭证验证与连接隔离**：`/api/media-providers` 接口支持 `connected=true` 服务端过滤。
- **CSS 注释解析警告消除**：消除 Lightning CSS 的 `Unexpected token Delim('*')` 构建警告。

[1.1.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.0.0...v1.1.0
[1.2.0]: https://github.com/arisvia/cyrene-gateway/compare/v1.1.0...v1.2.0
[1.0.0]: https://github.com/arisvia/cyrene-gateway/releases/tag/v1.0.0
