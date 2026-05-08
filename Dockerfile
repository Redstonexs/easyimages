FROM golang:1.21-alpine AS builder

# 安装依赖
RUN apk add --no-cache gcc musl-dev

# 设置工作目录
WORKDIR /build

# 复制go.mod和go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 编译
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o easyimage .

# 最终镜像
FROM alpine:latest

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /build/easyimage .

# 复制静态资源
COPY --from=builder /build/public ./public
COPY --from=builder /build/config ./config
COPY --from=builder /build/admin ./admin

# 创建必要目录
RUN mkdir -p /app/i/cache /app/i/suspic /app/i/recycle

# 设置权限
RUN chmod +x /app/easyimage

# 创建自动迁移脚本
RUN echo '#!/bin/sh' > /app/docker-entrypoint.sh && \
    echo '' >> /app/docker-entrypoint.sh && \
    echo '# 检测PHP配置并自动迁移' >> /app/docker-entrypoint.sh && \
    echo 'if [ -f /app/config/config.php ] && [ ! -f /app/config/config.json ]; then' >> /app/docker-entrypoint.sh && \
    echo '    echo "检测到PHP版本配置，开始自动迁移..."' >> /app/docker-entrypoint.sh && \
    echo 'fi' >> /app/docker-entrypoint.sh && \
    echo '' >> /app/docker-entrypoint.sh && \
    echo '# 启动应用' >> /app/docker-entrypoint.sh && \
    echo 'exec /app/easyimage' >> /app/docker-entrypoint.sh && \
    chmod +x /app/docker-entrypoint.sh

# 暴露端口
EXPOSE 8080

# 启动命令
CMD ["/app/docker-entrypoint.sh"]
