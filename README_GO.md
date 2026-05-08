# EasyImage Go版本

简单图床的Go语言重构版本，完全兼容PHP版本的数据和配置。

## 特性

- ✅ 完全兼容PHP版本的图片路径和URL
- ✅ 支持从PHP配置无缝迁移
- ✅ 高性能Go语言实现
- ✅ Docker部署支持
- ✅ 响应式Web界面
- ✅ API接口支持
- ✅ 图片压缩和缩略图
- ✅ 水印功能
- ✅ 用户认证系统
- ✅ 管理后台

## 快速开始

### 方式一：直接运行

```bash
# 编译
go build -o easyimage

# 运行
./easyimage
```

访问 http://localhost:8080 进行安装配置。

### 方式二：Docker部署

```bash
# 构建并运行
docker-compose up -d
```

### 方式三：从PHP版本迁移

```bash
# 1. 编译配置转换工具
cd cmd/php2json
go build -o php2json

# 2. 转换配置文件
./php2json ../../config/config.php ../../config/config.json

# 3. 编译并运行
cd ../../
go build -o easyimage
./easyimage
```

详细迁移说明请参考：[从PHP迁移到Go版本](docs/从PHP迁移到Go版本.md)

## 配置说明

配置文件位于 `config/config.json`，主要配置项：

```json
{
  "title": "站点标题",
  "domain": "http://your-domain.com",
  "imgurl": "http://your-domain.com",
  "user": "admin",
  "password": "sha256_hashed_password",
  "path": "/i/",
  "maxSize": 10485760,
  "port": 8080
}
```

## 目录结构

```
├── main.go                 # 入口文件
├── config/
│   ├── config.go          # 配置管理
│   └── config.json        # 配置文件
├── internal/
│   ├── handler/           # HTTP处理器
│   ├── middleware/         # 中间件
│   └── service/           # 业务逻辑
├── templates/             # HTML模板
├── public/                # 静态资源
├── i/                     # 图片存储
└── admin/                 # 管理后台日志
```

## API接口

### 上传图片

```bash
curl -X POST http://localhost:8080/api/index.php \
  -F "image=@/path/to/image.jpg" \
  -F "token=your_api_token"
```

### 响应格式

```json
{
  "result": "success",
  "code": 200,
  "url": "http://your-domain.com/i/2024/01/01/xxx.jpg",
  "srcName": "original_name",
  "thumb": "http://your-domain.com/app/thumb.php?img=/i/2024/01/01/xxx.jpg"
}
```

## Docker部署

### Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -o easyimage .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /build/easyimage .
COPY --from=builder /build/public ./public
COPY --from=builder /build/config ./config
EXPOSE 8080
CMD ["./easyimage"]
```

### docker-compose.yml

```yaml
version: '3.8'
services:
  easyimage:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./config:/app/config
      - ./i:/app/i
      - ./public:/app/public
```

## 开发

### 依赖

- Go 1.21+
- gin-gonic/gin
- disintegration/imaging

### 编译

```bash
go mod tidy
go build -o easyimage
```

## 许可证

GPL-2.0

## 致谢

- 原PHP版本作者 [icret](https://github.com/icret/EasyImages2.0)
- [gin-gonic/gin](https://github.com/gin-gonic/gin)
- [disintegration/imaging](https://github.com/disintegration/imaging)
