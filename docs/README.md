# EasyImage

EasyImage 当前以 Go 版本为主，保留 PHP 配置解析能力用于旧版本迁移。程序不依赖数据库，图片存储在 `i/`，日志存储在 `admin/logs/`，站点配置由安装流程生成到 `config/config.json`。

## 快速开始

```bash
go build -o easyimage .
./easyimage
```

访问 `http://localhost:8080/install/` 完成初始化。

Docker Compose：

```bash
docker-compose up -d
```

## 常用入口

- 首页：`/`
- 安装页：`/install/`
- 管理登录：`/admin/index`
- 管理后台：`/admin/manager`
- API 上传：`POST /api/index`

## 从 PHP 迁移

复制旧版本配置后启动 Go 服务即可自动迁移：

```bash
cp /path/to/php/config/config.php config/
cp /path/to/php/config/config.guest.php config/
cp /path/to/php/config/api_key.php config/
./easyimage
```

详情见 [从PHP迁移到Go版本](./从PHP迁移到Go版本.md)。

## 文档目录

- [安装图床](./安装图床.md)
- [安全配置](./安全配置.md)
- [API](./API.md)
- [三方安装指南](./三方安装指南.md)
- [图床更新升级](./图床更新升级.md)
- [常见问题](./常见问题.md)
- [许可证](./许可证.md)
