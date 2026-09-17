# AGENTS.md

> AI 编码智能体在此仓库协同工作时的参考规约。

## 项目总览

**Cyrene Gateway** 是 Go 1.26+ 编写的高性能 LLM API 网关，内置 Solid.js 单页控制台：
- `cmd/gateway/main.go`：服务启动入口与装配
- `internal/handler/`：OpenAI/Anthropic 兼容路由与管理 API
- `internal/provider/`：上游协议适配、凭证轮换、OAuth、SSRF 防护与代理池
- `internal/db/`：基于 `modernc.org/sqlite` 的 WAL 数据库存储
- `internal/config/`：配置解析（支持 `-data-dir`）
- `webui/`：基于 Solid.js + Vite + Tailwind CSS v4 的管理控制台

## 常用命令

```sh
# 后端检查与测试
go build ./... && go vet ./... && go test -count=1 ./...

# 前端类型检查、测试与打包
cd webui && npm run typecheck && npm test && npm run build
```

## 代码规范与约束

1. **类型安全**：前端严禁 `any` / `as any`，使用 `unknown` + 类型守卫或标准领域接口。
2. **样式层纪律**：Tailwind CSS v4 基础元素重置与全局样式必须置于 `@layer base`。
3. **出站安全**：出站 HTTP 必须通过 `SafeHTTPClient` 实施 SSRF 校验，阻断私网与云元数据逃逸。
4. **提交规约**：遵循 Conventional Commits（`feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`），提交粒度单一、原子化。
5. **动态模型同步**：禁止在 `registry_data.go` 硬编码静态模型列表（离线/专有端点除外），统一配置 `ModelsURL` 或动态抓取器保持与上游目录同步。
6. **UI 组件一致性**：
   - 空状态统一复用 `@/components/ui` 的 `<Empty />`，禁止页面私造 DOM。
   - 禁止在业务组件内联 Raw `<svg>`，统一收口至 `@/components/ui` (`icons.tsx`)。
   - 禁止使用裸 HTML 表单元素，统一使用 UI 导出的 `Input`、`Button`、`Select`、`Alert`。
7. **反重复造轮子（Deduplication）**：
   - Base64URL、PKCE 签名统一收口至 `provider/oauth.go`，UUID 生成统一使用 `uuid.New().String()`。
   - SSE 数据行解析统一使用 `translator.ParseSSEDataLine` 或 `ParseSSEDataLineString`。
   - 前端通用辅助函数统一收口至 `@/lib/format` 与 `@/lib/clipboard`。
   - 提供商凭据解析与免密模型路由必须统一走 `provider.ResolveCredentials`，严禁在 handler 散落手写凭据覆盖分支。
8. **全量国际化（Strict i18n）**：
   - 禁止在 JSX/TSX 中硬编码自然语言字符串。
   - 文案必须在 `zh-CN.ts` 与 `en-US.ts` 严格对齐，通过 `NestedKeyOf` 静态类型检查。
9. **极简架构与反代码堆砌（Anti-Bloat）**：
   - 严禁编写无引用抽象、单实现接口或空壳工厂（坚持 YAGNI）。
   - 优先平台原生与标准库方案；重构废弃旧逻辑时必须干净切割（Clean Cutover）。
10. **系统级端口交接与容器防呆**：
    - 自重启必须先关闭旧 Listener 释放端口，再启动带 `CYRENE_RESTART=1` 的子进程重试绑定。
    - 二进制物理覆盖与重启操作必须先经 `system.IsRunningInDocker()` 探测，容器内阻断并提示拉取镜像。
11. **本地预提交与 CI 门禁一致性（Strict CI Parity）**：
    - 每次 commit 之前，必须在本地完整跑通全套门禁：
      ```sh
      gofmt -w .
      if [ -n "$(gofmt -l .)" ]; then echo "Unformatted Go code found:"; gofmt -d .; exit 1; fi
      go vet ./...
      cd webui && npm run typecheck && npm test && npm run build && cd ..
      go test -count=1 ./...
      go test -race ./...
      go build ./...
      ```
12. **UI 排版与交互防抖纪律（Layout Stability & Focus Ring）**：
    - 凡 `overflow-y-auto` 容器必须使用对称水平内边距（如 `px-1`、`px-1.5`），预留 `focus:ring` 阴影缓冲区。
    - 转场容器的 `overflow-x-hidden` 仅在动画过渡期间动态激活，常态保持 `visible`。
    - 工作台类页面（如演练场）的空状态必须与顶部选择栏、底部输入框保持平齐垂直轴线，禁止多层嵌套缩进与无关边框。
    - 动态悬浮展示的标签或数值提示，其父容器必须锁定固定高度/尺寸，并使用透明度渐变（`opacity`）控制显隐，严禁使用导致父容器高度突变的动态装载（避免 CLS 布局抖动）；柱状图等序列控件的清空重置事件必须绑定于整体外层容器，禁止单个子项触发间隙抖动。
13. **发布与文档极简纪律（Lean Changelog & Zero Noise）**：
    - `CHANGELOG.md` 仅收录高价值核心要点（一级加粗标题 + 单句说明），严禁下挂嵌套二三级子项或代码实现级流水账。
    - CI `release.yml` 必须精确截取单版本日志并实施 Fail-Closed（未找到即失败），严禁回退全量 Changelog。
    - 严禁在代码仓库遗留临时草稿或废弃设计文档；敏感凭证严禁提交至代码库。
