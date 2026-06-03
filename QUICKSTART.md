# EasyImage 快速开始

## 环境要求

- Go 1.21+
- Node.js 22+，用于构建前端资源
- 可选：Docker / Docker Compose
- 可选：`cwebp`，用于启用 WebP 转换

## 全新安装

```bash
git clone https://github.com/Redstonexs/easyimages.git
cd easyimages
npm ci
npm run build
go build -o easyimage .
./easyimage
```

Windows：

```powershell
go build -o easyimage.exe .
.\easyimage.exe
```

打开 `http://localhost:8080/install/` 完成初始化。安装完成后会生成：

- `config/config.json`
- `config/install.lock`

这些文件是运行态配置，已在 `.gitignore` 中排除。

## Docker Compose

```bash
git clone https://github.com/Redstonexs/easyimages.git
cd easyimages
docker-compose up -d
docker-compose logs -f
```

默认映射端口为 `8080:8080`，运行态目录通过卷挂载：

- `./config:/app/config`
- `./i:/app/i`
- `./admin/logs:/app/admin/logs`

## 从 PHP 版本迁移

先备份旧站点的 `config/` 和 `i/` 目录。然后把旧 PHP 配置复制到本仓库：

```bash
cp /path/to/php/config/config.php config/
cp /path/to/php/config/config.guest.php config/
cp /path/to/php/config/api_key.php config/
npm ci
npm run build
go build -o easyimage .
./easyimage
```

自动迁移会生成 Go JSON 配置，并将 PHP 配置备份到 `config/php_backup/`。

如果需要手动转换主配置：

```bash
go run ./cmd/php2json config/config.php config/config.json
```

检查迁移状态：

```bash
go run ./cmd/migrate_test
```

完整迁移说明见 [docs/从PHP迁移到Go版本.md](docs/从PHP迁移到Go版本.md)。

## 验证

- 首页：`http://localhost:8080/`
- 管理登录：`http://localhost:8080/admin/index`
- 管理后台：`http://localhost:8080/admin/manager`
- API 上传：`POST http://localhost:8080/api/index`

API 示例：

```bash
curl -X POST http://localhost:8080/api/index \
  -F "image=@/path/to/image.jpg" \
  -F "token=your_api_token"
```

## 常用开发命令

```bash
go test ./...
go vet ./...
npm run typecheck
npm run build
go build -o easyimage .
```
