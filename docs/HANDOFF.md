# Cross-Session Handoff

每个 Agent session 结束前必须更新本文件的“最近一次交接”区块，并保持简短。

## 开始工作

1. 读取 `progress.json` 的 `current_work_item`。
2. 读取该 work item 的 `spec`。
3. 读取 `docs/ARCHITECTURE_DECISIONS.md` 与相关审计。
4. 检查工作区是否有未提交修改。
5. 先运行该模块现有测试，记录 baseline。
6. 只处理一个 work item，不顺手开启下一项。

## 完成工作

- 更新实现和测试
- 更新 API contract 或产品文档
- 执行 work item 的验证命令
- 在 `progress.json` 更新 status、summary、verification
- 将 `current_work_item` 指向下一个 pending work item
- 在下方记录修改、验证、剩余风险和下一步

## 阻塞规则

如果环境缺少 Go、Node、浏览器或网络依赖，不得把 work item 标记 done。将状态设为 `blocked`，记录准确原因和恢复命令，不得声称测试通过。

## 最近一次交接

- 日期：2026-08-13
- 当前工作项：37B（37A 已完成）
- 已完成（37A）：
  - 默认仅监听 loopback（`127.0.0.1`）；非 loopback 绑定强制全部 `/api/*` 会话认证（含 tunnel/MITM/CLI 等高权限接口），首次设置密码前保留 bootstrap 通道。
  - 管理 API 全面脱敏：Connection/APIKey/Node DTO（`hasApiKey`/`hasRefreshToken`/`credentialHint` 等），list/detail/mutation 均不返回凭据；字段级 patch 防止掩码回写覆盖凭据；settings 响应不含 `passwordHash`，PUT/PATCH 不可注入。
  - 删除默认密码 `123456`，Argon2id 哈希 + 首次设置流程（前端 setup 屏）；密码最短 8 位。
  - 登录 limiter 改为 mutex 保护、容量上限 10k、过期清理的实例化 `LoginLimiter`；`go test -race` 通过。
  - auth secret 移除 `init()`，`main` 中显式 `InitSecretManager(cfg.DataDir, cfg.Secret)`，文件 0600、目录 0700。
  - usage history 响应不再序列化调用方 API key（`json:"-"`）。
  - 前端适配：`ConnectionData` 类型、key 一次性展示、详情页 `hasRefreshToken`、登录错误态修复。
- 验证：go fmt/vet/test/test -race/build 全绿；webui npm ci/test/build 全绿；浏览器走查覆盖首次设置、登录/登出、key 一次性展示、添加 Provider、脱敏详情、字段级 patch、light 主题、错误态。
- 环境限制：沙箱浏览器无法调整视口，375px 走查未执行（属 38B/38D 验收，非 37A）。
- 尚未完成：Phase 37 其余 work item（37B 起）。
- 下一步：实施 37B Unified Upstream Executor。
