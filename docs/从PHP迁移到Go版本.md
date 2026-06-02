# 从 PHP 版本迁移到 Go 版本

本文说明如何把旧 PHP 版 EasyImage 迁移到当前 Go 版本。Go 版本保留图片目录结构和主要配置含义，但当前实际路由不带 `.php` 后缀。

## 迁移前备份

先在旧站点备份运行态数据：

```bash
cp -r config/ config_backup/
cp -r i/ i_backup/
cp -r admin/logs/ admin_logs_backup/
```

## 自动迁移

把旧 PHP 配置复制到当前仓库的 `config/` 目录：

```bash
cp /path/to/php/config/config.php config/
cp /path/to/php/config/config.guest.php config/
cp /path/to/php/config/api_key.php config/
go build -o easyimage .
./easyimage
```

Docker Compose：

```bash
cp /path/to/php/config/config.php config/
cp /path/to/php/config/config.guest.php config/
cp /path/to/php/config/api_key.php config/
docker-compose up -d
docker-compose logs -f
```

启动时会执行：

1. 检测可迁移的 PHP 配置。
2. 备份 PHP 配置到 `config/php_backup/`。
3. 解析 PHP 数组配置。
4. 生成 `config/config.json`、`config/config.guest.json`、`config/api_key.json`。
5. 创建 `config/install.lock`。
6. 确保 `i/` 和 `admin/logs/` 运行目录存在。

仓库内置的默认 `config/*.php` 只作为迁移模板，不会触发自动迁移。只有复制了真实旧站点配置后才会自动迁移。

## 手动转换

自动迁移失败时，可以先手动转换主配置：

```bash
go run ./cmd/php2json config/config.php config/config.json
```

检查迁移状态：

```bash
go run ./cmd/migrate_test
```

## 兼容范围

保留兼容：

- 图片目录：`/i/YYYY/MM/DD/file.ext`
- 缩略图缓存、回收站、可疑图片目录
- 管理员用户名和旧 SHA256 密码哈希
- 上传者配置：`config/config.guest.php`
- API token 配置：`config/api_key.php`
- 主要上传限制、图片处理和安全配置项

当前 Go 路由：

| 功能 | Go 路由 |
| --- | --- |
| 首页 | `/` |
| 安装页 | `/install/` |
| 管理登录 | `/admin/index` |
| 管理后台 | `/admin/manager` |
| Web 上传 | `POST /app/upload` |
| API 上传 | `POST /api/index` |
| 缩略图 | `/app/thumb?img=/i/...` |

旧 PHP 风格的 `/api/index.php`、`/admin/index.php`、`/admin/manager.php` 当前没有兼容路由。外部工具需要改成 `/api/index`。

## 配置对照

| PHP 配置项 | JSON 配置项 | 说明 |
| --- | --- | --- |
| `$config['title']` | `title` | 站点标题 |
| `$config['domain']` | `domain` | 站点域名 |
| `$config['imgurl']` | `imgurl` | 图片域名 |
| `$config['user']` | `user` | 管理员用户名 |
| `$config['password']` | `password` | 管理员密码哈希 |
| `$config['path']` | `path` | 图片 URL 路径 |
| `$config['storage_path']` | `storage_path` | 存储目录格式 |
| `$config['maxSize']` | `maxSize` | 单文件大小限制 |
| `$config['maxUploadFiles']` | `maxUploadFiles` | 单次上传数量限制 |
| `$config['extensions']` | `extensions` | 允许扩展名 |
| `$config['compress']` | `compress` | 是否压缩 |
| `$config['compress_ratio']` | `compress_ratio` | 压缩质量 |
| `$config['thumbnail']` | `thumbnail` | 缩略图开关 |
| `$config['watermark']` | `watermark` | 水印开关 |
| `$config['mustLogin']` | `mustLogin` | 是否登录后上传 |
| `$config['apiStatus']` | `apiStatus` | API 上传开关 |
| `$config['check_ip']` | `check_ip` | IP 限制开关 |
| `$config['md5_black']` | `md5_black` | MD5 黑名单开关 |
| `$config['hide']` | `hide` | 源图保护开关 |
| `$config['image_recycl']` | `image_recycl` | 回收站开关 |
| `$config['timezone']` | `timezone` | 时区 |

## 验证迁移

访问管理后台：

```text
http://your-domain.com/admin/manager
```

测试 API：

```bash
curl -X POST http://your-domain.com/api/index \
  -F "image=@test.jpg" \
  -F "token=your_api_token"
```

检查旧图片：

```text
http://your-domain.com/i/2024/01/01/xxx.jpg
```

## 回滚

如果需要回滚到 PHP 版本：

```bash
pkill easyimage
cp config/php_backup/config.php config/
cp config/php_backup/config.guest.php config/
cp config/php_backup/api_key.php config/
rm config/config.json config/config.guest.json config/api_key.json config/install.lock
```

然后按旧 Web 服务器配置重新启动 PHP 版本。

## 常见问题

### 自动迁移没有触发

确认复制的是旧站点真实 `config/*.php`，不是仓库内置默认模板；同时确认不存在已安装的 `config/config.json` 和 `config/install.lock`。

### API 调用失败

确认后台已开启 API 上传，token 未过期，并且请求地址是 `/api/index`。

### 迁移后无法登录

清除浏览器 Cookie 后重试。Go 版本支持旧 SHA256 密码，也支持安装后保存的新 bcrypt 密码。
