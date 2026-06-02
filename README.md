# EasyImage

[![Go](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/Redstonexs/easyimages)](https://github.com/Redstonexs/easyimages/releases)
[![License](https://img.shields.io/badge/license-GPL_V2.0-yellowgreen.svg)](LICENSE)

EasyImage 是一个无数据库图床服务。当前仓库已以 Go 版本为主：一个 Gin 二进制、Go HTML 模板、文件配置和文件存储；保留 PHP 配置文件仅用于从旧版本迁移。

## 主要特性

- 单二进制部署，无需 PHP 运行环境
- 无数据库，图片默认存储在 `i/`
- 支持 Web 上传、粘贴上传和 API 上传
- 支持管理员后台、上传历史、图片列表和文件管理
- 支持缩略图、压缩、水印、WebP 转换和热链保护
- 支持从旧 PHP 配置自动迁移到 Go JSON 配置
- 支持 Docker / Docker Compose 部署

## 快速开始

```bash
go build -o easyimage .
./easyimage
```

Windows 下运行 `easyimage.exe`。服务默认监听 `:8080`，首次安装访问：

```text
http://localhost:8080/install/
```

Docker Compose：

```bash
docker-compose up -d
```

更多步骤见 [QUICKSTART.md](QUICKSTART.md)。

## 常用路由

- 首页：`GET /`
- 安装页：`GET /install/`
- 管理登录：`GET /admin/index`
- 管理后台：`GET /admin/manager`
- Web 上传：`POST /app/upload`
- API 上传：`POST /api/index`
- 缩略图：`GET /app/thumb?img=/i/...`

API 示例：

```bash
curl -X POST http://localhost:8080/api/index \
  -F "image=@/path/to/image.jpg" \
  -F "token=your_api_token"
```

## 目录说明

```text
.
├── main.go                  # 应用入口、路由和启动逻辑
├── config/                  # Go 配置代码和 PHP 迁移输入
├── internal/                # handler、middleware、service
├── templates/               # Go HTML 模板
├── public/                  # 静态资源
├── cmd/php2json/            # 手动 PHP 配置转换工具
├── cmd/migrate_test/        # 迁移状态检查工具
├── docs/                    # 用户文档
├── i/                       # 运行时图片存储，除 .gitkeep 外不提交
└── admin/logs/              # 运行时日志，不提交
```

运行态文件不会提交到仓库：`config/config.json`、`config/config.guest.json`、`config/api_key.json`、`config/install.lock`、`config/php_backup/`、`admin/logs/` 和 `i/`。

## 配置和迁移

全新安装时无需手写配置，安装流程会生成 `config/config.json` 和 `config/install.lock`。

从 PHP 版本迁移时，把旧站点的以下文件复制到 `config/` 后启动 Go 服务：

```bash
cp /path/to/php/config/config.php config/
cp /path/to/php/config/config.guest.php config/
cp /path/to/php/config/api_key.php config/
./easyimage
```

启动时会解析 PHP 数组配置，生成 Go JSON 配置，并把原 PHP 配置备份到 `config/php_backup/`。完整说明见 [从PHP迁移到Go版本](docs/从PHP迁移到Go版本.md)。

## 开发命令

```bash
go test ./...
go vet ./...
go build -o easyimage .
go run ./cmd/php2json config/config.php config/config.json
go run ./cmd/migrate_test
```

WebP 转换依赖外部 `cwebp` 命令。Docker 镜像已安装 `libwebp-tools`，本地运行需要自行安装并放到 `PATH`。

## 文档

- [快速开始](QUICKSTART.md)
- [API 文档](docs/API.md)
- [安装图床](docs/安装图床.md)
- [Docker 部署](docs/三方安装指南.md)
- [从 PHP 迁移到 Go 版本](docs/从PHP迁移到Go版本.md)
- [常见问题](docs/常见问题.md)

## 许可证

GPL-2.0。原 PHP 版本作者为 [icret/EasyImages2.0](https://github.com/icret/EasyImages2.0)。
