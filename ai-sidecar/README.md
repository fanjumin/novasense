# novasense-ai sidecar — 用法速览

NovaSense 的 AI 推理旁路（FastAPI + ONNX Runtime，CPU 基线；RKNN 为后续镜像 tag）。
契约与改造点见仓库根 `docs/AI-UPGRADE-PLAN.md`（本服务是缝A/缝B 的执行端）。

## 端点

| 方法 | 路径 | 用途 | 失败语义 |
|---|---|---|---|
| GET | `/v1/healthz` | 就绪探测（Go `/api/ai/status` 透传） | — |
| POST | `/v1/judge` | 运动快照判帧（YOLO11n） | 409=模型未载（Go 弃权走原逻辑） |
| POST | `/v1/embed_face` | 人脸 512d 向量（ArcFace r100） | 同上 |

鉴权：`Authorization: Bearer $AI_INTERNAL_TOKEN`（与 Gateway 同一 token；未配 token 视为内网裸奔，日志告警）。

## 跑起来

```bash
# 1. 权重(不入 git): 下载/导出并回填 models.lock.json 的 sha256
MODEL_DIR=./models ./fetch_models.sh

# 2. 本地直跑(开发)
pip install -r requirements.txt
AI_INTERNAL_TOKEN=dev python -m uvicorn app.main:app --port 8901

# 3. 生产: 随 gateway/docker-compose.yml 的 `ai` 服务一起起
#    (仅发布 127.0.0.1:8901; Gateway 容器 host 网络直连)
```

## Gateway 侧开关（环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `AI_SIDECAR_URL` | `http://127.0.0.1:8901`(compose) | 置空 = 关闭全部 AI 缝，行为回到 v0.8.4 |
| `AI_MODE` | `shadow` | shadow=只记录不拦截（灰度观察）；enforce=允许降级背景误报 |
| `AI_INTERNAL_TOKEN` | 必填 | 与 sidecar 一致才生效 |
| `AI_TIMEOUT_MS` | `800` | 超时=弃权（fail-open，绝不阻塞运动管线） |

## 环境变量（sidecar 自身）

`AI_INTERNAL_TOKEN` · `MODEL_DIR` · `AI_DETECT_CLASSES`(默认 person) · `AI_DETECT_CONF`(0.35) · `AI_NMS_IOU`(0.45) · `AI_MAX_IMAGE_BYTES`

## 测试

```bash
python -m pytest tests -q   # 9 项：YOLO 解码/类别过滤/NMS + 鉴权/409/413/422 契约
```

## embedding 存储格式（与 Go 对齐）

512 × float32 小端字节序 BLOB；`faces.emb_model` 记模型标识。换模型 = 全库重算（脚本化前勿混用）。

## 许可提醒

YOLO11 权重为 AGPL-3.0（ultralytics）。对外分发/商用前评估：换 Apache-2.0 的
RT-DETR / YOLO-NAS 导出即可，接口契约不变。ArcFace(buffalo) 权重为非商用研究许可，
生产需换合规人脸模型。
