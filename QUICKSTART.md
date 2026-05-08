# EasyImage Go版本 - 快速开始

## 一分钟快速上手

### 方式一：全新安装

```bash
# 1. 克隆项目
git clone https://github.com/icret/EasyImages2.0.git
cd EasyImages2.0

# 2. 编译
go build -o easyimage

# 3. 运行
./easyimage

# 4. 访问 http://localhost:8080/install/ 进行配置
```

### 方式二：从PHP版本迁移（自动）

```bash
# 1. 克隆项目
git clone https://github.com/icret/EasyImages2.0.git
cd EasyImages2.0

# 2. 复制PHP配置文件
cp /path/to/php/config/config.php config/
cp /path/to/php/config/config.guest.php config/
cp /path/to/php/config/api_key.php config/

# 3. 编译
go build -o easyimage

# 4. 运行（自动迁移）
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

### 方式三：Docker部署（自动迁移）

```bash
# 1. 克隆项目
git clone https://github.com/icret/EasyImages2.0.git
cd EasyImages2.0

# 2. 复制PHP配置文件
cp /path/to/php/config/config.php config/

# 3. 启动Docker
docker-compose up -d

# 4. 查看日志
docker-compose logs -f
```

## 验证迁移

### 1. 测试迁移配置

```bash
# 编译迁移测试工具
cd cmd/migrate_test
go build -o migrate_test

# 运行测试
./migrate_test
```

### 2. 检查配置文件

```bash
# 查看生成的配置文件
cat config/config.json

# 查看PHP配置备份
ls config/php_backup/
```

### 3. 访问管理后台

访问 `http://localhost:8080/admin/manager.php` 查看配置是否正确。

## 常见问题

### Q: 自动迁移失败怎么办？

A: 使用手动迁移：
```bash
cd cmd/php2json
go build -o php2json
./php2json ../../config/config.php ../../config/config.json
```

### Q: 如何重新迁移？

A: 删除Go配置文件后重新运行：
```bash
rm config/config.json
./easyimage
```

### Q: 如何回滚到PHP版本？

A: 恢复备份的PHP配置：
```bash
cp config/php_backup/config.php config/
rm config/config.json
```

## 详细文档

- [完整迁移指南](docs/从PHP迁移到Go版本.md)
- [图床更新升级](docs/图床更新升级.md)
- [常见问题](docs/常见问题.md)
- [API文档](docs/API.md)

## 技术支持

- GitHub Issues: https://github.com/icret/EasyImages2.0/issues
- Telegram: https://t.me/Easy_Image
