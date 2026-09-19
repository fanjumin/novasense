#!/bin/bash
# NovaSense Gateway 裸机启动脚本（相对路径，无个人环境依赖）
# 用法：先 go build -o novasense-gateway . 再 ./run.sh
set -euo pipefail
cd "$(dirname "$0")"

BIN=./novasense-gateway
[ -x "$BIN" ] || { echo "未找到可执行文件 $BIN，请先执行: go build -o novasense-gateway ."; exit 1; }

export DATA_DIR="${DATA_DIR:-$(pwd)/data}"
LOG_DIR="${LOG_DIR:-/tmp}"
mkdir -p "$DATA_DIR"

exec "$BIN" >> "$LOG_DIR/novasense-gateway-stdout.log" 2>> "$LOG_DIR/novasense-gateway-stderr.log"
