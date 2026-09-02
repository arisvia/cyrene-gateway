# 多阶段构建：Node 构建面板 → Go 构建静态二进制 → 最小运行镜像

# ---------- 1) 面板构建 ----------
FROM node:24-alpine AS webui
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /src/webui
COPY webui/package.json webui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY webui/ ./
RUN pnpm build

# ---------- 2) Go 构建 ----------
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY webui/embed.go webui/embed.go
COPY --from=webui /src/webui/dist webui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/cyrene-gateway ./cmd/gateway

# ---------- 3) 运行镜像 ----------
FROM alpine:3.21
# ca-certificates: 出站 HTTPS 需要；curl: /api/health 健康检查
RUN apk add --no-cache ca-certificates curl tzdata && adduser -D -u 10001 gateway
COPY --from=build /out/cyrene-gateway /usr/local/bin/cyrene-gateway
USER gateway
ENV CYRENE_HOST=0.0.0.0
ENV CYRENE_PORT=20128
# 数据目录挂载点（SQLite 数据库与面板缓存落盘）
VOLUME ["/home/gateway/.cyrene-gateway"]
EXPOSE 20128
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:20128/api/health || exit 1
ENTRYPOINT ["/usr/local/bin/cyrene-gateway"]
