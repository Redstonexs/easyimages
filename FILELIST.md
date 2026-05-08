# EasyImage Go版本 - 项目文件清单

## 核心文件

### 入口文件
- `main.go` - Go程序入口文件，包含自动迁移逻辑

### 配置管理
- `config/config.go` - 配置管理代码
- `config/php_migrate.go` - PHP配置解析和自动迁移
- `config/config.json` - JSON格式配置文件（自动生成）
- `config/config.guest.json` - 上传者配置（自动生成）
- `config/api_key.json` - API密钥配置（自动生成）

### HTTP处理
- `internal/handler/handler.go` - HTTP处理器
- `internal/middleware/middleware.go` - 中间件

### 业务逻辑
- `internal/service/service.go` - 业务逻辑
- `internal/service/image.go` - 图片处理

## 模板文件

### HTML模板
- `templates/index.html` - 首页
- `templates/list.html` - 图片列表
- `templates/info.html` - 图片信息
- `templates/error.html` - 错误页面
- `templates/install.html` - 安装页面
- `templates/admin_login.html` - 管理登录
- `templates/admin_manager.html` - 管理后台
- `templates/admin_filer.html` - 文件管理
- `templates/admin_chart.html` - 统计页面

## 静态资源

### CSS样式
- `public/static/zui/css/zui.min.css` - ZUI框架样式
- `public/static/EasyImage.css` - 自定义样式

### JavaScript
- `public/static/zui/js/zui.min.js` - ZUI框架脚本
- `public/static/zui/lib/jquery/jquery.js` - jQuery库
- `public/static/zui/lib/clipboard/clipboard.min.js` - 剪贴板库
- `public/static/viewjs/viewer.min.js` - 图片预览库
- `public/static/viewjs/viewer.min.css` - 图片预览样式
- `public/static/lazyload/lazyload.min.js` - 懒加载库

### 图片资源
- `public/images/loading.svg` - 加载动画
- `public/images/file.svg` - 文件图标

## Docker相关

### Docker配置
- `Dockerfile` - Docker构建文件（支持自动迁移）
- `docker-compose.yml` - Docker Compose配置（支持自动迁移）
- `docker-entrypoint.sh` - Docker入口脚本（自动迁移）

## 迁移工具

### 配置转换
- `cmd/php2json/main.go` - PHP配置转JSON工具
- `cmd/migrate_test/main.go` - 迁移测试工具

## 文档

### 项目文档
- `README.md` - 主项目说明
- `README_GO.md` - Go版本说明
- `QUICKSTART.md` - 快速开始指南
- `SUMMARY.md` - 项目总结

### 迁移文档
- `docs/从PHP迁移到Go版本.md` - 完整迁移指南
- `docs/图床更新升级.md` - 更新升级说明
- `docs/安装图床.md` - 安装说明
- `docs/三方安装指南.md` - Docker部署说明
- `docs/常见问题.md` - 常见问题

### 其他文档
- `docs/API.md` - API文档
- `docs/安全配置.md` - 安全配置
- `docs/鉴黄.md` - 鉴黄功能
- `docs/隐藏存储路径.md` - 隐藏路径
- `docs/_sidebar.md` - 侧边栏导航
- `docs/SUMMARY.md` - 文档目录

## 配置文件

### Go模块
- `go.mod` - Go模块文件
- `go.sum` - Go依赖校验（需要运行go mod tidy生成）

## 目录结构

```
F:\easyimage\
├── main.go                    # 入口文件
├── go.mod                     # Go模块
├── Dockerfile                 # Docker构建
├── docker-compose.yml         # Docker Compose
├── docker-entrypoint.sh       # Docker入口脚本
├── config/
│   ├── config.go              # 配置管理
│   ├── php_migrate.go         # PHP迁移
│   ├── config.json            # Go配置
│   ├── config.guest.json      # 上传者配置
│   ├── api_key.json           # API密钥
│   └── php_backup/            # PHP配置备份
├── internal/
│   ├── handler/
│   │   └── handler.go         # HTTP处理
│   ├── middleware/
│   │   └── middleware.go      # 中间件
│   └── service/
│       ├── service.go         # 业务逻辑
│       └── image.go           # 图片处理
├── templates/
│   ├── index.html             # 首页
│   ├── list.html              # 列表
│   ├── info.html              # 信息
│   ├── error.html             # 错误
│   ├── install.html           # 安装
│   ├── admin_login.html       # 管理登录
│   ├── admin_manager.html     # 管理后台
│   ├── admin_filer.html       # 文件管理
│   └── admin_chart.html       # 统计
├── public/
│   ├── static/
│   │   ├── zui/               # ZUI框架
│   │   ├── viewjs/            # 图片预览
│   │   ├── lazyload/          # 懒加载
│   │   └── EasyImage.css      # 自定义样式
│   └── images/                # 图片资源
├── cmd/
│   ├── php2json/              # PHP配置转换
│   └── migrate_test/          # 迁移测试
├── docs/
│   ├── 从PHP迁移到Go版本.md   # 迁移指南
│   ├── 图床更新升级.md         # 升级说明
│   └── ...                    # 其他文档
├── i/                         # 图片存储
│   ├── cache/                 # 缩略图缓存
│   ├── suspic/                # 可疑图片
│   └── recycle/               # 回收站
├── admin/
│   └── logs/
│       ├── upload/            # 上传日志
│       └── ipcounts/          # IP计数
└── public/                    # 静态资源
```

## 文件数量统计

- Go源文件: 7个
- HTML模板: 9个
- CSS文件: 3个
- JavaScript文件: 6个
- 配置文件: 5个
- 文档文件: 15个
- Docker文件: 3个
- 工具文件: 2个

总计: 约50个文件
