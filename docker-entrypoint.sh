#!/bin/sh

# EasyImage Docker 入口脚本
# 自动检测PHP配置并执行迁移

echo "========================================"
echo "EasyImage Docker 启动脚本"
echo "========================================"

# 检查是否存在PHP配置
if [ -f /app/config/config.php ]; then
    echo "检测到PHP版本配置文件: config/config.php"
    
    # 检查是否已有Go配置
    if [ ! -f /app/config/config.json ]; then
        echo "未检测到Go版本配置，开始自动迁移..."
        echo "========================================"
        
        # 备份PHP配置
        if [ ! -d /app/config/php_backup ]; then
            mkdir -p /app/config/php_backup
            cp /app/config/config.php /app/config/php_backup/ 2>/dev/null || true
            cp /app/config/config.guest.php /app/config/php_backup/ 2>/dev/null || true
            cp /app/config/api_key.php /app/config/php_backup/ 2>/dev/null || true
            echo "PHP配置已备份到 config/php_backup/ 目录"
        fi
        
        # 确保目录存在
        mkdir -p /app/i/cache /app/i/suspic /app/i/recycle
        mkdir -p /app/admin/logs/upload /app/admin/logs/ipcounts
        
        echo "自动迁移将在应用启动时执行..."
        echo "========================================"
    else
        echo "已存在Go版本配置，跳过迁移"
        echo "========================================"
    fi
else
    echo "未检测到PHP版本配置"
    
    # 检查是否需要首次配置
    if [ ! -f /app/config/config.json ]; then
        echo "未检测到Go版本配置，将进入安装向导"
        echo "请访问 http://localhost:8080/install/ 进行配置"
        echo "========================================"
    fi
fi

# 确保必要目录存在
mkdir -p /app/i/cache /app/i/suspic /app/i/recycle
mkdir -p /app/admin/logs/upload /app/admin/logs/ipcounts

# 启动应用
echo "启动EasyImage应用..."
exec /app/easyimage
