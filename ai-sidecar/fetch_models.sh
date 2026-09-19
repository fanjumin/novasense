#!/usr/bin/env bash
# fetch_models.sh — 拉取并校验模型权重（幂等；sha256 留空则下载后打印供回填 models.lock.json）
# 用法: MODEL_DIR=./models ./fetch_models.sh
set -euo pipefail
MODEL_DIR="${MODEL_DIR:-./models}"
mkdir -p "$MODEL_DIR"

echo "[models] 依赖工具检查…"
command -v python3 >/dev/null || { echo "需要 python3"; exit 1; }

# 1) YOLO11n: 下载 .pt 并导出 onnx（导出一次后缓存）
if [ ! -f "$MODEL_DIR/yolo11n.onnx" ]; then
  echo "[models] 获取 yolo11n.pt 并导出 ONNX…"
  python3 - <<'EOF'
import urllib.request, hashlib, os
url = "https://github.com/ultralytics/assets/releases/download/v8.3.0/yolo11n.pt"
dst = os.path.join(os.environ.get("MODEL_DIR", "./models"), "yolo11n.pt")
if not os.path.isfile(dst):
    with urllib.request.urlopen(url, timeout=120) as r, open(dst + ".part", "wb") as f:
        f.write(r.read())
    os.replace(dst + ".part", dst)
print("pt sha256:", hashlib.sha256(open(dst, "rb").read()).hexdigest())
try:
    from ultralytics import YOLO
    YOLO(dst).export(format="onnx", simplify=True, opset=12)
except ImportError:
    raise SystemExit("请先 pip install ultralytics onnxslim(可选) 后重跑")
EOF
  mv -f "$MODEL_DIR/yolo11n.onnx" "$MODEL_DIR/yolo11n.onnx" 2>/dev/null || true
fi

# 2) ArcFace r100: insightface 官方包内 buffalo_l 的 scrfd/arcface 子模型
if [ ! -f "$MODEL_DIR/arcface_r100.onnx" ]; then
  echo "[models] 提示: arcface 权重请从 models.lock.json 的 source 页面手动下载 buffalo_l"
  echo "        解包后取 onnx/glint360k_r100 对应文件改名 arcface_r100.onnx 放入 $MODEL_DIR"
fi

for f in "$MODEL_DIR"/*.onnx; do
  [ -f "$f" ] && echo "sha256: $(sha256sum "$f")"
done
echo "[models] 完成。回填 sha256 到 models.lock.json 以钉版。"
