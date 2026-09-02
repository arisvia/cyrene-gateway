# Cyrene Gateway 前端重构蓝图（SolidJS + Vite 8 + Tailwind 4）

> **状态：线框稿（待用户确认后实施）**
> 目标：借鉴 9router 面板的简洁布局，功能全保留（MITM/Tunnel 简化呈现），砍掉死掉的 i18n。
> 本文档为重构唯一依据，实施前需确认。

## 1. 技术栈

| 项 | 选择 | 说明 |
|---|---|---|
| 框架 | SolidJS 1.9 | 细粒度响应式，无 VDOM，正是网关面板这种中小应用的甜点区 |
| 构建 | Vite 8 + vite-plugin-solid | 用户指定 |
| 样式 | Tailwind 4（@tailwindcss/vite 插件） | 9router 同款；语义 token（bg-bg/text-muted/border-subtle）+ 自研 Button/Card/Badge/Modal 微组件，**不用重型 UI 库** |
| 路由 | @solidjs/router | hash 模式（现有后端 SPA 回退无扩展名约束，hash 最稳） |
| 状态 | @solidjs/store createStore + context | 不引第三方状态库 |
| 图表 | 自绘 SVG 面积图（~100 行）或 recharts-solid 兼容层；先自绘 | 9router 用 recharts，但我们 KPI 图需求简单，自绘零依赖 |
| 类型检查 | typescript ~5.9 + vite build（不做 vue-tsc 式强类型检查门禁） | 吸取本次构建断裂教训：类型检查独立成 `npm run typecheck`，不阻塞构建 |
| 测试 | vitest + @solidjs/testing-library | 迁移现有 4 个测试文件 |

## 2. 信息架构（左侧固定侧栏，两级分组）

```
┌────────────────┬──────────────────────────────────────────┐
│ LOGO cyrene    │  Header: 全局搜索(占位) · 版本徽章 · 主题切换 │
│ v{version}     ├──────────────────────────────────────────┤
│                │                                          │
│ 接入            │            主内容区 max-w-7xl            │
│  ▸ 首页/端点    │            居中 · p-6 lg:p-10            │
│  ▸ 提供商       │                                          │
│  ▸ 组合 Combo  │                                          │
│  ▸ 用量         │                                          │
│  ▸ 配额         │                                          │
│                │                                          │
│ 系统            │                                          │
│  ▾ 媒体         │                                          │
│    图像/语音/…  │                                          │
│  ▸ 代理池       │                                          │
│  ▸ CLI 工具     │                                          │
│  ▸ 控制台       │                                          │
│  ▸ 隧道         │                                          │
│  ▸ MITM        │                                          │
│                │                                          │
│ 设置            │                                          │
└────────────────┴──────────────────────────────────────────┘
```

- **移动端**：侧栏变滑入抽屉 + 遮罩（9router 同款）
- **Token Saver** 并入「设置」页（单开关级别功能，不值独立页）；路由 `/token-saver` 保留重定向
- 导航组：`接入`（首页/提供商/组合/用量/配额）+ `系统`（媒体/代理池/CLI 工具/控制台/隧道/MITM）+ `设置`

## 3. 页面线框

### 3.1 首页 `/`
```
┌ 网关状态卡 ────────────────────────────────────┐
│ ● 运行中   v1.2.3   运行 3 天   5 个活跃连接    │
└───────────────────────────────────────────────┘
┌ 端点卡片（点击复制）─┐ ┌ 快速接入 ────────────┐
│ OpenAI   /v1/...   │ │ Claude Code [配置]   │
│ Anthropic /v1/...  │ │ Codex       [配置]   │
│ 网关地址 Tailscale │ │ …更多工具 → CLI 工具页│
└────────────────────┘ └─────────────────────┘
┌ 技能网格（/api/skills）─ 6 卡 ┐
```

### 3.2 提供商 `/providers`（重头戏，按 9router 卡片式重写）
```
工具栏: [添加连接·向导] [测试全部] [启用免费] [批量导入]    搜索框
────────────────────────────────────────────────────────
▼ Anthropic (2 连接)                                    [收起]
┌──────────────────────────────────────────────────────┐
│ ● 主账号 claude.ai    [ACTIVE]  优先级 10  用量 ▓▓░  [测][⋯]│
│ ○ 备用账号            [429 冷却 3m] 优先级 20          │
└──────────────────────────────────────────────────────┘
▼ OpenAI (1)   ▼ Gemini (1)   … 按品牌分组折叠，默认收起非首个
状态徽章语义: ACTIVE(绿) / COOLDOWN(黄+剩余时间) / EXPIRED(红) / UNTESTED(灰)
```
- 每行整行可点进详情；开关、测试、重置是行内动作
- OAuth 设备码/轮询流程做成统一 Modal（现有 1067 行 Vue 里的核心交互）

### 3.3 提供商详情 `/providers/:id`
```
头部: logo + 名称 + 状态徽章 + [测试][刷新模型][刷新 OAuth][重置]
┌ 概览卡: 类型 OAuth/Key · 优先级 · 冷却至 · 最近错误(红字) ┐
┌ 模型表: model · 目录来源 · 启用 ┐  ┌ 该连接用量卡 ┐
```

### 3.4 用量 `/usage`（9router KPI 布局）
```
[Overview | Details]   周期: [24h|7d|30d]
┌────────┬────────┬────────┬──────────┬─────────┐
│ 总请求  │ Prompt │ Cached │ Completion│ 预估成本 │
│  1,204 │ 3.2M   │ 810K   │ 512K     │ $4.21   │
└────────┴────────┴────────┴──────────┴─────────┘
┌ 面积图: [Tokens|成本] 切换 · 按天 ─ 自绘 SVG ┐
┌ 最近请求实时表(30s 轮询): 时间·模型·状态·tokens·耗时 ┐
```
Details 段: 按 模型/提供商/Key/端点 四维透视表（URL 排序）。

### 3.5 配额 `/quota`
```
[自动刷新 60s 倒计时] [立即刷新]
账号表: 提供商 · 连接 · 配额进度条 · 剩余 · 冷却计时 · [单测]
```

### 3.6 组合 `/combos`
卡片列表: 名称 · 策略徽章(fallback/round-robin) · 模型链 · 内联编辑 Modal。

### 3.7 隧道 `/tunnel`（**简化**：单卡）
```
┌ Tailscale ──────────────────────────────┐
│ 状态: 未安装 / 运行中(https://xxx.ts.net) │
│ [安装] [开启 Funnel] [关闭]              │
└─────────────────────────────────────────┘
```

### 3.8 MITM `/mitm`（**简化**：单卡 + 流量表）
```
┌ MITM 代理 ──────────────────────────────┐
│ 状态: 停止   [启动][停止]  CA 指纹(复制)  │
└─────────────────────────────────────────┘
┌ 捕获流量表: host · 方法 · 状态 · 耗时 ┐
```

### 3.9 设置 `/settings`
```
§ 安全: 面板密码 · requireApiKey 开关 · apiKeyRpm 数字输入(新)
§ 组合策略: fallback / round-robin
§ 关于: 版本 · LICENSE · 链接
```

### 3.10 其余
- 媒体 `/media/:kind`：侧栏手风琴 6 子项（embedding/image/video/tts/stt/web），内容 = 提供商网格 + 试跑表单
- CLI 工具 `/cli-tools` 卡片网格 → 详情 `/cli-tools/:id`
- 控制台 `/console`：slog 日志流表
- 代理池、技能：卡片列表

## 4. 目录结构（新）

```
webui/src/
  index.tsx            # 入口
  App.tsx              # Layout: Sidebar + Header + <Outlet/>
  lib/
    api.ts             # 平移（不变）
    format.ts toast.ts # 平移
  stores/
    gateway.ts         # createStore 重写（信号化，保留 27 端点封装）
  components/
    ui/                # Button/Card/Badge/Modal/Toast/Switch/Skeleton
    layout/            # Sidebar Header
  pages/               # 一页一文件（对应上节线框）
    Home.tsx Providers.tsx ProviderDetail.tsx Usage.tsx Quota.tsx
    Combos.tsx Media.tsx CliTools.tsx CliToolDetail.tsx Console.tsx
    ProxyPools.tsx Tunnel.tsx Mitm.tsx Skills.tsx Settings.tsx
  styles/tokens.css    # 语义 token 平移（bg-bg/text-muted/border-subtle）
```

## 5. 迁移与验收

1. **删除** `webui/src` 全部 Vue 代码 + `vue`/`pinia`/`vue-router`/`vue-tsc` 依赖 + `vitest.config.ts` 中 Vue 插件；`i18n/` 目录删除
2. 新栈从零搭骨架 → 布局 → 按 3.1–3.10 顺序逐页迁移
3. `npm run build` 产出 `dist/` → `go build` embed → 起服人工过一遍 16 页
4. `npm test` 迁移 4 个测试（api/format/gateway/connection）
5. 全量 `go test ./...` 回归（后端不动）

## 6. 风险

- `ProvidersView`（1067 行）是最大迁移单元，OAuth 设备码轮询等异步交互最易错 → 单独一节实施 + 人工过流程
- Tailwind 4 的 vite 插件要求 Node ≥ 20（已满足）
- embed 白屏回归风险：验收第 3 步必须做「干净克隆构建」复验
