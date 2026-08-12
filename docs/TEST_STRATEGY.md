# Test Strategy

## 测试金字塔

### 单元测试

- Route resolution
- fallback classification
- credential selection
- auth injection
- translation
- SSE event parser
- DTO redaction
- URL/SSRF policy
- password/session/limiter
- 前端 formatter、stores、wizard state machine、SSE client

### Contract 与组件测试

- 管理 API response 永不出现 secret
- Provider Catalog connectionMethods schema
- Connection CRUD 与局部 mutation response
- Vue Connection Wizard 的各认证模式
- Connection Workspace 的 loading/error/empty 状态

### E2E

最少覆盖：

1. 首次启动与安全初始化
2. 添加 API Key Connection，测试并保存
3. 完成 OAuth 或 device-code 模拟流程
4. 添加 NoAuth Provider
5. OpenAI 非流式请求
6. OpenAI SSE 请求与客户端取消
7. Anthropic/Gemini 转换
8. Combo 429 fallback 与 400 不 fallback
9. Route Inspector 与实际选择一致
10. 删除 Connection 后资源和路由状态同步

## 安全测试

- 匿名管理 API
- secret response contract
- 登录 limiter 并发与容量
- 请求体、multipart、header、响应体上限
- SSRF IPv4/IPv6、DNS rebind 思路、redirect chain
- zip-slip、zip bomb 边界、panel 完整性
- 敏感文件权限与原子写入

## 发布命令

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...

govulncheck ./...

cd webui
npm ci
npm test
npm run build
```

浏览器验收必须覆盖 dark/light、桌面、375px、键盘、reduced motion，以及空、加载、失败、长文本和慢请求。
