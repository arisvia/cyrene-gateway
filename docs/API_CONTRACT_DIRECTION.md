# API Contract Direction

Phase 37A 与 38A 实施时以本文为方向，可直接破坏旧接口，不保留兼容层。

## 原则

1. URL、DTO 与前端类型使用 Connection，而不是把 ProviderConnection 暴露为 Provider。
2. 持久化模型永不直接序列化为 response。
3. Secret 写入 DTO 与读取 DTO 分离。
4. 错误返回稳定的 `code`、`message`、`details` 和 `requestId`。
5. List response 统一 `{items, pagination?}`，不再同时兼容数组和多种包装结构。
6. Mutation response 返回更新后的脱敏 resource，减少前端全量刷新。
7. 所有异步流程有明确状态，OAuth/device code 不依赖页面猜测。

## 建议资源

```text
GET    /api/provider-catalog
GET    /api/provider-catalog/{providerId}
GET    /api/connections
POST   /api/connections
GET    /api/connections/{connectionId}
PATCH  /api/connections/{connectionId}
DELETE /api/connections/{connectionId}
POST   /api/connections/{connectionId}/test
POST   /api/connections/{connectionId}/refresh
GET    /api/connections/{connectionId}/models
GET    /api/routing/resolve?model=...
```

## Secret response

Connection response 只能包含：

```text
hasApiKey
hasAccessToken
hasRefreshToken
credentialHint
expiresAt
```

不能包含：

```text
apiKey
accessToken
refreshToken
cookie
provider-specific secret
```

## Connection 创建流程

1. 从 Provider Catalog 获取 `connectionMethods` schema。
2. 前端提交 method ID 与对应 fields。
3. 后端验证 schema、测试连接并创建 Connection。
4. 后端返回脱敏 Connection DTO。

可将“测试后保存”实现成 draft/validate/commit，也可由一次事务型 create 完成，但不能先保存无效凭据再依赖用户手动测试。
