# 从PHP版本迁移到Go版本

本文档说明如何将EasyImage从PHP版本迁移到Go版本，确保数据和配置的无缝迁移。

## 自动迁移功能

Go版本支持**自动迁移**功能，当检测到PHP版本配置时会自动执行迁移。

### 自动迁移流程

1. **检测PHP配置** - 启动时自动检测是否存在PHP配置文件
2. **备份PHP配置** - 将PHP配置备份到 `config/php_backup/` 目录
3. **解析PHP配置** - 自动解析PHP数组格式的配置文件
4. **转换为JSON格式** - 将PHP配置转换为Go版本的JSON格式
5. **迁移数据目录** - 确保图片目录结构完整
6. **创建安装锁** - 标记迁移完成

### 自动迁移触发条件

- 存在 `config/config.php` 文件
- 不存在 `config/config.json` 文件

### 自动迁移支持的配置

- ✅ 站点配置（标题、域名、描述等）
- ✅ 管理员配置（用户名、密码）
- ✅ 上传配置（大小限制、格式限制等）
- ✅ 图片处理配置（压缩、缩略图、水印等）
- ✅ 安全配置（IP黑白名单、MD5黑名单等）
- ✅ 上传者配置（config/guest.php）
- ✅ API密钥配置（config/api_key.php）

## 迁移方式

### 方式一：直接运行（自动迁移）

```bash
# 1. 克隆项目
git clone https://github.com/icret/EasyImages2.0.git
cd EasyImages2.0

# 2. 复制PHP配置文件到config目录
cp /path/to/php/config/config.php config/
cp /path/to/php/config/config.guest.php config/
cp /path/to/php/config/api_key.php config/

# 3. 编译Go版本
go build -o easyimage

# 4. 运行（自动检测并迁移）
./easyimage
```

启动时会显示：
```
========================================
检测到PHP版本配置，开始自动迁移...
========================================
自动迁移完成！
PHP配置已备份到 config/php_backup/ 目录
========================================
```

### 方式二：Docker部署（自动迁移）

```bash
# 1. 克隆项目
git clone https://github.com/icret/EasyImages2.0.git
cd EasyImages2.0

# 2. 复制PHP配置文件到config目录
cp /path/to/php/config/config.php config/
cp /path/to/php/config/config.guest.php config/
cp /path/to/php/config/api_key.php config/

# 3. 使用Docker Compose启动
docker-compose up -d

# 4. 查看日志
docker-compose logs -f
```

Docker容器启动时会自动执行迁移脚本。

### 方式三：手动迁移

如果自动迁移失败，可以手动执行：

```bash
# 1. 编译配置转换工具
cd cmd/php2json
go build -o php2json

# 2. 转换配置文件
./php2json ../../config/config.php ../../config/config.json
./php2json ../../config/config.guest.php ../../config/config.guest.json
./php2json ../../config/api_key.php ../../config/api_key.json

# 3. 编译并运行
cd ../../
go build -o easyimage
./easyimage
```

## 目录结构

Go版本保持与PHP版本相同的目录结构：

```
├── i/                      # 图片存储目录（保持不变）
│   ├── 2024/              # 按年份的图片目录
│   │   ├── 01/            # 按月份
│   │   │   ├── 01/        # 按日期
│   │   │   │   ├── xxx.jpg
│   ├── cache/             # 缩略图缓存
│   ├── suspic/            # 可疑图片
│   └── recycle/           # 回收站
├── public/                # 静态资源
├── config/                # 配置文件
│   ├── config.json        # Go版本配置（自动生成）
│   ├── config.guest.json  # 上传者配置（自动生成）
│   ├── api_key.json       # API密钥配置（自动生成）
│   └── php_backup/        # PHP配置备份
└── admin/
    └── logs/
        ├── upload/        # 上传日志
        └── ipcounts/      # IP计数
```

## 数据兼容性

### 1. 图片URL兼容

Go版本保持与PHP版本相同的图片URL格式：

- PHP版本：`http://domain.com/i/2024/01/01/xxx.jpg`
- Go版本：`http://domain.com/i/2024/01/01/xxx.jpg`

**无需修改任何图片链接！**

### 2. API接口兼容

API端点保持一致：

- 上传接口：`/api/index.php`
- 响应格式：JSON格式不变

### 3. 管理后台兼容

管理后台路径保持一致：

- 登录页面：`/admin/index.php`
- 管理页面：`/admin/manager.php`

### 4. 密码兼容

Go版本使用相同的SHA256密码哈希方式，PHP版本的密码可以直接使用。

## 配置对照表

| PHP配置项 | JSON配置项 | 说明 | 默认值 |
|-----------|------------|------|--------|
| `$config['title']` | `title` | 站点标题 | 简单图床 - EasyImage |
| `$config['domain']` | `domain` | 站点域名 | http://127.0.0.1:8080 |
| `$config['imgurl']` | `imgurl` | 图片域名 | http://127.0.0.1:8080 |
| `$config['user']` | `user` | 管理员用户名 | admin |
| `$config['password']` | `password` | 管理员密码哈希 | - |
| `$config['path']` | `path` | 存储路径 | /i/ |
| `$config['storage_path']` | `storage_path` | 目录格式 | Y/m/d/ |
| `$config['maxSize']` | `maxSize` | 最大上传大小(字节) | 10485760 |
| `$config['maxUploadFiles']` | `maxUploadFiles` | 最大上传数量 | 30 |
| `$config['extensions']` | `extensions` | 允许的扩展名 | jpg,jpeg,png,gif... |
| `$config['compress']` | `compress` | 是否压缩 | 0 |
| `$config['compress_ratio']` | `compress_ratio` | 压缩质量 | 80 |
| `$config['thumbnail']` | `thumbnail` | 缩略图模式 | 1 |
| `$config['thumbnail_w']` | `thumbnail_w` | 缩略图宽度 | 258 |
| `$config['thumbnail_h']` | `thumbnail_h` | 缩略图高度 | 258 |
| `$config['watermark']` | `watermark` | 水印类型 | 0 |
| `$config['waterText']` | `waterText` | 水印文字 | - |
| `$config['waterPosition']` | `waterPosition` | 水印位置 | 9 |
| `$config['mustLogin']` | `mustLogin` | 必须登录 | 0 |
| `$config['apiStatus']` | `apiStatus` | API状态 | 0 |
| `$config['check_ip']` | `check_ip` | IP检查 | 0 |
| `$config['check_ip_list']` | `check_ip_list` | IP名单 | - |
| `$config['md5_black']` | `md5_black` | MD5黑名单 | 0 |
| `$config['upload_logs']` | `upload_logs` | 上传日志 | 0 |
| `$config['hide']` | `hide` | 源图保护 | 0 |
| `$config['hide_key']` | `hide_key` | 保护密钥 | EasyImage2.0 |
| `$config['hide_path']` | `hide_path` | 隐藏路径 | 0 |
| `$config['admin_path_status']` | `admin_path_status` | 管理员目录 | 0 |
| `$config['admin_path']` | `admin_path` | 管理员目录名 | u |
| `$config['guest_path_status']` | `guest_path_status` | 上传者目录 | 0 |
| `$config['token_path_status']` | `token_path_status` | Token目录 | 0 |
| `$config['image_recycl']` | `image_recycl` | 回收站 | 1 |
| `$config['show_user_hash_del']` | `show_user_hash_del` | 用户删除 | 1 |
| `$config['listNumber']` | `listNumber` | 列表数量 | 20 |
| `$config['listDate']` | `listDate` | 列表日期 | 10 |
| `$config['timezone']` | `timezone` | 时区 | Asia/Shanghai |
| `$config['port']` | `port` | 端口 | 8080 |

## 迁移后验证

### 1. 检查配置

访问管理后台，确认配置正确：

```
http://your-domain.com/admin/manager.php
```

### 2. 测试上传

上传一张图片，确认功能正常：

```
http://your-domain.com/
```

### 3. 测试API

使用原有API密钥测试上传：

```bash
curl -X POST http://your-domain.com/api/index.php \
  -F "image=@test.jpg" \
  -F "token=your_api_token"
```

### 4. 检查图片访问

确认原有图片可以正常访问：

```
http://your-domain.com/i/2024/01/01/xxx.jpg
```

## 回滚方案

如果需要回滚到PHP版本：

1. 停止Go版本服务
2. 恢复备份的PHP文件
3. 恢复备份的配置文件
4. 启动PHP版本服务

```bash
# 停止Go版本
pkill easyimage

# 恢复PHP配置
cp config/php_backup/config.php config/
cp config/php_backup/config.guest.php config/
cp config/php_backup/api_key.php config/

# 删除Go配置
rm config/config.json config/config.guest.json config/api_key.json

# 启动PHP版本（根据你的Web服务器配置）
```

## 常见问题

### Q: 自动迁移失败怎么办？

A: 可以使用手动迁移方式：
```bash
cd cmd/php2json
go build -o php2json
./php2json ../../config/config.php ../../config/config.json
```

### Q: 迁移后图片无法访问？

A: 检查以下配置：
- `path` 配置是否正确
- `domain` 和 `imgurl` 是否正确
- 图片目录权限是否正确

### Q: 迁移后无法登录？

A: 检查以下配置：
- `user` 和 `password` 是否正确复制
- Cookie是否过期，尝试清除浏览器Cookie

### Q: API调用失败？

A: 检查以下配置：
- `apiStatus` 是否为 1（开启）
- API密钥是否正确转换
- Token是否过期

### Q: 缩略图无法生成？

A: 检查以下目录：
- `i/cache/` 目录是否存在
- 目录权限是否正确

### Q: 上传失败？

A: 检查以下配置：
- `maxSize` 是否正确
- `extensions` 是否包含所需格式
- 目录权限是否正确

## 技术支持

如有问题，请通过以下方式获取支持：

- GitHub Issues: https://github.com/icret/EasyImages2.0/issues
- Telegram: https://t.me/Easy_Image
