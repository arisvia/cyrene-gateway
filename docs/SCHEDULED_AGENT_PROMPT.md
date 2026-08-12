# Scheduled Agent Bootstrap Prompt

下列内容可作为每日定时新 Session 的初始消息。仓库内文档是任务和架构的 source of truth；本消息只负责安全地初始化环境和驱动一个 work item。

---

你正在执行 Cyrene Gateway 的一次无人值守定时开发 Session。目标是安全、可验证地推进一个 work item，而不是完成整个 phase。

## 0. 不可违反的规则

- 每次 Session 最多完成 `progress.json.current_work_item` 指向的一个 work item。
- 不跨 work item，不顺手开发 roadmap 项，不新增 Provider。
- 项目未投产，不保留旧 API、旧字段、旧数据库或旧 WebUI 兼容层。
- 未实际执行的测试不得写成“通过”。缺少工具或依赖时必须标记 blocked。
- 不输出、提交或记录 `.env`、token、cookie、API key、Authorization header 等 secret。
- 不直接修改或提交历史归档文件，除非任务明确要求归档。
- 上游 9router/VansRouter 仅作为行为参考，不得覆盖本仓库当前架构决策。

## 1. 安全初始化运行环境

```bash
set -euo pipefail
umask 077
mkdir -p /data/workspace /data/toolcache
```

如果 `/data/.env` 存在，加载它但不得打印其内容：

```bash
if [ -f /data/.env ]; then
  set -a
  . /data/.env
  set +a
fi
```

验证必要变量，不在错误信息中输出变量值：

```bash
: "${GITHUB_REPO:?GITHUB_REPO is required}"
: "${GITHUB_USER:?GITHUB_USER is required}"
: "${GITHUB_EMAIL:?GITHUB_EMAIL is required}"
```

`GITHUB_REPO` 必须是 `owner/repo` 格式。项目目录使用仓库 basename，避免把斜杠直接拼进目录：

```bash
case "$GITHUB_REPO" in
  */*) ;;
  *) echo "GITHUB_REPO must be owner/repo" >&2; exit 1 ;;
esac
REPO_NAME="${GITHUB_REPO##*/}"
PROJECT_DIR="/data/workspace/$REPO_NAME"
```

配置 Git 身份：

```bash
git config --global user.name "$GITHUB_USER"
git config --global user.email "$GITHUB_EMAIL"
```

### Go

仓库要求的 Go 版本以 `go.mod` 为准。当前基线为 Go 1.26.x。优先使用已有满足要求的工具链；不得仅因为 patch 版本不同就反复覆盖系统目录。

如果 Go 不存在或 major/minor 低于 1.26，在 `/data/toolcache` 安装经过 SHA-256 校验的官方 Linux archive，并将其 `bin` 放到当前 Session PATH。不要删除 `/usr/local/go`，除非该沙箱明确保证此目录只属于本任务。

当前已知可用基线为 Go 1.26.5，但安装前仍应从官方 release metadata 获取对应文件名和 SHA-256，不使用未校验的 `curl | tar` 管道。

安装后执行：

```bash
go version
go env GOTOOLCHAIN
```

### Node.js

WebUI 要求 Node >=22.12，优先使用 Node 24 Active LTS。若现有版本满足要求，不要重装。若需要安装，在 `/data/toolcache` 安装官方 archive并校验 `SHASUMS256.txt`，不要清空系统级 `/usr/local/lib/node_modules`。

安装后执行：

```bash
node --version
npm --version
```

## 2. 获取代码并建立干净 baseline

若项目不存在则 clone；存在则安全更新。禁止 `reset --hard` 覆盖未知工作：

```bash
if [ ! -d "$PROJECT_DIR/.git" ]; then
  git clone "https://github.com/$GITHUB_REPO.git" "$PROJECT_DIR"
fi
cd "$PROJECT_DIR"

if [ -n "$(git status --porcelain)" ]; then
  echo "Working tree is not clean; refusing unattended overwrite" >&2
  exit 1
fi

git fetch --prune origin
git checkout main
git pull --ff-only origin main
```

确认本次拿到的是 development-ready 文档结构：

```bash
test -f AGENTS.md
test -f progress.json
test -f docs/WORK_ITEMS.md
test -f docs/HANDOFF.md
```

首先阅读：

1. `AGENTS.md`
2. `progress.json`
3. `docs/HANDOFF.md`
4. `docs/WORK_ITEMS.md` 中 `current_work_item` 对应章节
5. `docs/ARCHITECTURE_DECISIONS.md`
6. current work item 引用的领域、API、测试和审计文档

检查 `current_work_item`、依赖和状态。若它不存在、已 done、或依赖未完成，则记录 blocked 并停止，不猜测下一项。

### Baseline

在修改代码前运行与当前 work item 相关的现有测试。后端任务至少运行：

```bash
go test ./...
```

涉及 WebUI 时运行：

```bash
cd webui
npm ci
npm test
npm run build
cd ..
```

不要在 Session 开头无条件运行 `go mod tidy`。只有实际修改 Go 依赖时才运行，并检查 `go.mod`/`go.sum` diff。

如果 baseline 已失败，先判断是否属于当前 work item。无关失败应记录 blocked，不把已有失败误归因于本次修改。

## 3. 执行当前 work item

- 只实现 `docs/WORK_ITEMS.md` 中当前 work item 的 Scope。
- 严格遵守 Out of scope 与 Acceptance。
- 先补失败测试或 contract test，再修改实现。
- API、领域术语和前端类型遵守：
  - `docs/DOMAIN_MODEL.md`
  - `docs/API_CONTRACT_DIRECTION.md`
- 测试遵守 `docs/TEST_STRATEGY.md`。
- 涉及 Provider 行为且确有必要时，再获取参考源码。

参考仓库按需获取，不在每个 Session 无条件 clone：

```bash
# 仅当当前任务需要 9router 行为对照时
if [ ! -d /data/workspace/9router/.git ]; then
  git clone --filter=blob:none --no-checkout https://github.com/decolua/9router.git /data/workspace/9router
fi

# 仅当 WORK_ITEMS 或审计明确要求 VansRouter 特有增强时获取
```

如果仓库文档记录了参考 commit，应 checkout 该 commit，避免每天的 upstream 变化导致实现漂移。

## 4. 验证

按当前 work item 执行全部 Acceptance。后端最终门禁至少包括：

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

如果 `govulncheck` 已安装，执行：

```bash
govulncheck ./...
```

涉及 WebUI 时：

```bash
cd webui
npm test
npm run build
cd ..
```

不得使用 `npx` 临时下载未锁定版本。使用 `npm ci` 根据 lockfile 安装并运行 package scripts。

WebUI 任务还必须实际启动应用并进行浏览器走查，覆盖 `docs/TEST_STRATEGY.md` 要求。若沙箱没有浏览器能力，将 work item 标记 blocked，不得以 build 代替交互验收。

`webui/dist` 的处理以仓库当前 ignore/commit 策略为准：

- 若 dist 被忽略，不提交它。
- 若仅占位文件被跟踪，构建验证后恢复占位文件，并确认没有误删其他已跟踪资源。
- 不要无条件执行“恢复 index.html”，先用 `git status` 和 `git ls-files webui/dist` 判断。

## 5. 更新状态与交接

只有所有 Acceptance 和验证门禁通过后：

- 将当前 work item `status` 设为 `done`
- 写一行 `summary`
- 在 `verification` 中记录实际执行且成功的命令
- 按依赖顺序把 `current_work_item` 指向下一个 pending item
- 只有一个 phase 的全部 work item 都 done 时，才将 phase 标记 done 并推进 `current_phase`
- 更新 `last_updated` 为实际日期
- 更新 `docs/HANDOFF.md` 的最近一次交接

若阻塞：

- 将当前 work item 设为 `blocked`
- 记录精准原因、失败命令和恢复条件
- 保持 `current_work_item` 不变
- 更新 `docs/HANDOFF.md`
- 不提交半成品行为，除非是安全的测试或文档诊断提交

更新后验证 JSON：

```bash
python3 -m json.tool progress.json >/dev/null
```

## 6. 提交与推送

提交前检查：

```bash
git status --short
git diff --check
git diff --stat
```

禁止提交 `.env`、数据库、日志、token、构建缓存和临时文件。

推荐使用 work item 分支并推送，由平台创建 PR：

```bash
git checkout -b "work/${CURRENT_WORK_ITEM,,}"
git add -A
git commit -m "fix: ${CURRENT_WORK_ITEM} - concise description"
git push -u origin HEAD
```

如果平台明确要求直接推送 main，仍需先确认远端未前进并使用普通 push，禁止 force push。

不要在每个 nightly work item 后打版本 tag。只有 Release Gate work item 或明确的 release 任务完成，并且 `progress.json` 指定版本号时才创建 tag。版本号不得由 phase 编号机械推导。

## 7. 最终报告

最终输出必须包含：

- work item ID 与最终状态
- 修改摘要
- 实际运行的验证命令及结果
- commit hash / branch / PR（若有）
- 剩余风险或阻塞
- 下一 work item

---
