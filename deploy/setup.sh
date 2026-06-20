#!/bin/bash
# setup.sh — 视频流管理平台 安装脚本
# 用法: sudo ./setup.sh [systemd|docker]

set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
MODE="${1:-systemd}"

echo "=== 视频流管理平台 安装脚本 ==="
echo "项目目录: $PROJECT_DIR"
echo "安装模式: $MODE"
echo ""

case "$MODE" in
  systemd)
    echo "[1/3] 复制 service 文件..."
    sudo cp "$PROJECT_DIR/deploy/mediamtx.service" /etc/systemd/system/
    sudo cp "$PROJECT_DIR/deploy/video-stream-manager.service" /etc/systemd/system/

    echo "[2/3] 重新加载 systemd..."
    sudo systemctl daemon-reload

    echo "[3/3] 启动服务..."
    sudo systemctl enable --now mediamtx
    sudo systemctl enable --now video-stream-manager

    echo ""
    echo "✅ 已安装 systemd 服务:"
    echo "   mediamtx.service         → :8888/:1935/:8554"
    echo "   video-stream-manager.service → :8899"
    echo ""
    echo "查看状态: sudo systemctl status mediamtx video-stream-manager"
    echo "查看日志: sudo journalctl -u mediamtx -f"
    echo "         sudo journalctl -u video-stream-manager -f"
    ;;

  docker)
    echo "[1/2] 构建 Docker 镜像..."
    cd "$PROJECT_DIR"
    docker compose build

    echo "[2/2] 启动服务..."
    docker compose up -d

    echo ""
    echo "✅ Docker Compose 已启动:"
    echo "   mediamtx  → :8888/:1935/:8554"
    echo "   app       → :8899"
    echo ""
    echo "查看日志: docker compose logs -f"
    echo "停止:     docker compose down"
    ;;

  *)
    echo "用法: $0 [systemd|docker]"
    exit 1
    ;;
esac
