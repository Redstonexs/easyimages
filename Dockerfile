FROM node:26.13.1-alpine3.24@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS frontend

RUN apk upgrade --no-cache

WORKDIR /build

COPY scripts/setup-npm.sh ./
RUN sh setup-npm.sh

COPY package.json package-lock.json* ./
RUN npm ci

COPY vite.config.ts tsconfig.json ./
COPY web ./web
RUN npm run build

FROM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

ARG VERSION=3.0.0

ENV GOTOOLCHAIN=local

RUN apk upgrade --no-cache

# 设置工作目录
WORKDIR /build

# 复制go.mod和go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .
COPY --from=frontend /build/public/dist ./public/dist

# 删除可能存在的默认配置文件（避免干扰迁移检测）
RUN rm -f config/config.json config/config.guest.json config/api_key.json config/install.lock

# 编译（通过ldflags注入版本号）
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w -X easyimage/config.Version=${VERSION}" -o easyimage .

# 最终镜像
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

# 安装运行时依赖（libwebp-tools 提供 cwebp 命令，用于 WebP 编码）
RUN apk upgrade --no-cache && apk add --no-cache \
    ca-certificates \
    tzdata \
    libwebp-tools \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /build/easyimage .

# 复制静态资源
COPY --from=builder /build/public ./public
COPY --from=builder /build/templates ./templates

# 复制PHP配置文件（用于自动迁移）
COPY --from=builder /build/config/config.php ./config/config.php
COPY --from=builder /build/config/config.guest.php ./config/config.guest.php
COPY --from=builder /build/config/api_key.php ./config/api_key.php

# 创建必要目录
RUN mkdir -p /app/i/cache /app/i/suspic /app/i/recycle && \
    mkdir -p /app/admin/logs/upload /app/admin/logs/ipcounts

# 设置权限
RUN chmod +x /app/easyimage

# 暴露端口
EXPOSE 8080

# 启动命令
CMD ["./easyimage"]
