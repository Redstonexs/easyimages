# EasyImage Go版本 - 迁移指南

## 从PHP版本迁移

### 1. 配置文件迁移

#### 方法一：使用转换工具

```bash
# 编译转换工具
cd cmd/php2json
go build -o php2json

# 转换配置文件
./php2json ../../config/config.php ../../config/config.json
./php2json ../../config/config.guest.php ../../config/config.guest.json
./php2json ../../config/api_key.php ../../config/api_key.json
```

#### 方法二：手动创建

直接编辑 `config/config.json` 文件，参考PHP版本的配置项。

### 2. 目录结构

保持以下目录结构不变：

```
├── i/                    # 图片存储目录
│   ├── cache/           # 缩略图缓存
│   ├── suspic/          # 可疑图片
│   └── recycle/         # 回收站
├── public/              # 静态资源
└── admin/
    └── logs/
        ├── upload/      # 上传日志
        └── ipcounts/    # IP计数
```

### 3. 数据兼容

- **图片URL**: 保持原有路径格式 `/i/2024/01/01/xxx.jpg`
- **API接口**: 端点保持一致 `/api/index.php`
- **管理后台**: 路径保持一致 `/admin/`

### 4. 部署方式

#### 方式一：直接运行

```bash
# 编译
go build -o easyimage

# 运行
./easyimage
```

#### 方式二：Docker部署

```bash
# 构建镜像
docker build -t easyimage .

# 运行容器
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config:/app/config \
  -v $(pwd)/i:/app/i \
  -v $(pwd)/public:/app/public \
  -v $(pwd)/admin/logs:/app/admin/logs \
  --name easyimage \
  easyimage
```

#### 方式三：Docker Compose

```bash
docker-compose up -d
```

### 5. 配置项对照表

| PHP配置 | JSON配置 | 说明 |
|---------|----------|------|
| `$config['title']` | `title` | 站点标题 |
| `$config['domain']` | `domain` | 站点域名 |
| `$config['imgurl']` | `imgurl` | 图片域名 |
| `$config['user']` | `user` | 管理员用户名 |
| `$config['password']` | `password` | 管理员密码(哈希) |
| `$config['path']` | `path` | 存储路径 |
| `$config['maxSize']` | `maxSize` | 最大上传大小 |
| `$config['extensions']` | `extensions` | 允许的扩展名 |
| `$config['compress_ratio']` | `compress_ratio` | 压缩质量 |
| `$config['thumbnail']` | `thumbnail` | 缩略图模式 |
| `$config['watermark']` | `watermark` | 水印类型 |

### 6. 主要改进

1. **性能提升**: Go语言原生性能优于PHP
2. **内存占用**: 更低的内存占用
3. **并发处理**: 原生支持高并发
4. **部署简单**: 单二进制文件，无依赖
5. **Docker支持**: 官方Docker镜像

### 7. 注意事项

1. Go版本使用JSON配置，不再使用PHP数组
2. 密码哈希方式保持兼容(SHA256)
3. Cookie认证方式保持兼容
4. 图片路径和URL格式保持一致

### 8. 回滚方案

如果需要回滚到PHP版本：

1. 保留原有的PHP文件
2. 配置文件可以使用转换工具反向转换
3. 图片目录结构不变，可直接使用

### 9. 常见问题

**Q: 为什么选择Go重写？**

A: Go语言具有更好的性能、更低的资源占用、更简单的部署方式。

**Q: 是否支持原有数据？**

A: 完全支持，图片和配置都可以无缝迁移。

**Q: API接口是否兼容？**

A: 完全兼容，原有的API调用无需修改。

**Q: 如何更新密码？**

A: 密码使用SHA256哈希，格式与PHP版本一致。
