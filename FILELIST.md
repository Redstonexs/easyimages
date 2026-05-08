# EasyImage Go版本 - 项目文件清单

## 核心文件

### Go源文件
- `main.go` - 入口文件（包含自动迁移逻辑）
- `config/config.go` - 配置管理
- `config/php_migrate.go` - PHP配置解析和自动迁移
- `internal/handler/handler.go` - HTTP处理器
- `internal/middleware/middleware.go` - 中间件
- `internal/service/service.go` - 业务逻辑
- `internal/service/image.go` - 图片处理

### Go模块
- `go.mod` - Go模块文件

## 配置文件

### PHP配置（用于自动迁移）
- `config/config.php` - 主配置文件
- `config/config.guest.php` - 上传者配置
- `config/api_key.php` - API密钥配置

### Go配置（自动生成）
- `config/config.json` - JSON格式配置（.gitignore）
- `config/config.guest.json` - 上传者配置（.gitignore）
- `config/api_key.json` - API密钥配置（.gitignore）

## HTML模板
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

### ZUI框架
- `public/static/zui/css/zui.min.css` - ZUI样式
- `public/static/zui/js/zui.min.js` - ZUI脚本
- `public/static/zui/lib/jquery/jquery-3.6.4.min.js` - jQuery库
- `public/static/zui/lib/clipboard/clipboard.min.js` - 剪贴板库
- `public/static/zui/lib/bootbox/` - 对话框库
- `public/static/zui/lib/datetimepicker/` - 日期选择器
- `public/static/zui/lib/chart/` - 图表库
- `public/static/zui/fonts/` - 字体文件
- `public/static/zui/theme/` - 主题样式

### 其他静态资源
- `public/static/viewjs/viewer.min.js` - 图片预览库
- `public/static/viewjs/viewer.min.css` - 图片预览样式
- `public/static/lazyload/lazyload.min.js` - 懒加载库
- `public/static/EasyImage.css` - 自定义样式
- `public/static/EasyImage.js` - 自定义脚本
- `public/static/login.css` - 登录页样式
- `public/static/crypto/SHA256.js` - SHA256加密
- `public/static/md5/` - MD5加密
- `public/static/echarts/` - ECharts图表
- `public/static/nprogress/` - 进度条
- `public/static/qrcode/` - 二维码生成
- `public/static/tinyfilemanager/` - 文件管理器

### 图片资源
- `public/images/` - 图片资源

## Docker相关
- `Dockerfile` - Docker构建文件（支持自动迁移）
- `docker-compose.yml` - Docker Compose配置（支持自动迁移）
- `docker-entrypoint.sh` - Docker入口脚本（自动迁移）

## 迁移工具
- `cmd/php2json/main.go` - PHP配置转JSON工具
- `cmd/migrate_test/main.go` - 迁移测试工具

## 文档
- `README.md` - 主项目说明
- `README_GO.md` - Go版本说明
- `QUICKSTART.md` - 快速开始指南
- `SUMMARY.md` - 项目总结
- `FILELIST.md` - 项目文件清单（本文件）
- `LICENSE` - GPL-2.0许可证
- `.gitignore` - Git忽略规则

### docs目录文档
- `docs/从PHP迁移到Go版本.md` - 迁移指南
- `docs/图床更新升级.md` - 更新升级说明
- `docs/安装图床.md` - 安装说明
- `docs/三方安装指南.md` - Docker部署说明
- `docs/常见问题.md` - 常见问题
- `docs/API.md` - API文档
- `docs/安全配置.md` - 安全配置
- 其他文档...

## 目录结构

```
F:\easyimage\
├── main.go                    # 入口文件
├── go.mod                     # Go模块
├── Dockerfile                 # Docker构建
├── docker-compose.yml         # Docker Compose
├── docker-entrypoint.sh       # Docker入口脚本
├── .gitignore                 # Git忽略规则
├── LICENSE                    # 许可证
├── README.md                  # 主说明文档
├── README_GO.md               # Go版本说明
├── QUICKSTART.md              # 快速开始
├── SUMMARY.md                 # 项目总结
├── FILELIST.md                # 文件清单
├── config/
│   ├── config.go              # 配置管理
│   ├── php_migrate.go         # PHP迁移
│   ├── config.php             # PHP配置（迁移用）
│   ├── config.guest.php       # 上传者配置（迁移用）
│   └── api_key.php            # API密钥（迁移用）
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
│   │   ├── EasyImage.css      # 自定义样式
│   │   └── EasyImage.js       # 自定义脚本
│   └── images/                # 图片资源
├── cmd/
│   ├── php2json/              # PHP配置转换
│   └── migrate_test/          # 迁移测试
├── docs/                      # 文档
└── i/                         # 图片存储
    └── .gitkeep
```
