#!/bin/bash
# 迁移旧的 data.json 到新的 SQLite 数据库
# 用法: ./scripts/migrate.sh
# 先启动后端，再运行此脚本

set -e

API="${1:-http://localhost:8899}"

if [ ! -f data/data.json ]; then
  echo "data/data.json 不存在，无需迁移"
  exit 0
fi

echo "正在从 data/data.json 导入数据到 SQLite..."

# 通过 ImportAll API 导入
curl -s -X PUT "${API}/api/import" \
  -H "Content-Type: application/json" \
  -d @"data/data.json" > /dev/null

echo "✅ 导入完成"

# 重命名旧文件作为备份
mv data/data.json data/data.json.bak
echo "旧文件已备份到 data/data.json.bak"
