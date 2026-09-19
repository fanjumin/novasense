"""test_smoke — 无真实权重下的可跑通验证：
1. decode_yolo 合成张量: 位置/类别/去重(NMS) 三条断言
2. API 契约: auth 401 / 模型缺失 409 / healthz 结构 / 图像 422/413
"""
import base64
import io
import os

os.environ.setdefault("AI_INTERNAL_TOKEN", "testtoken")
os.environ["MODEL_DIR"] = "/nonexistent-models-dir"  # 强制双模型均未就绪

import numpy as np
import pytest
from fastapi.testclient import TestClient

from app.detector import decode_yolo
from app.main import app


# ---------- 纯函数: YOLO 输出解码 ----------


def test_decode_yolo_single_person_box():
    # letterbox 640 → 原图 1280x720, scale=1280/640? 用 scale=2,pad=0 验映射
    out = np.zeros((1, 84, 8400), dtype=np.float32)
    cx, cy, w, h = 320.0, 240.0, 100.0, 200.0
    out[0, 0, 0] = cx
    out[0, 1, 0] = cy
    out[0, 2, 0] = w
    out[0, 3, 0] = h
    out[0, 4 + 0, 0] = 0.9  # class 0 = person
    dets = decode_yolo(out, conf_th=0.35, classes=["person"], iou_th=0.45,
                       img_w=1280, img_h=720, scale=2.0, pad_x=0, pad_y=0)
    assert len(dets) == 1
    d = dets[0]
    assert d["cls"] == "person" and abs(d["conf"] - 0.9) < 1e-6
    x1, y1, x2, y2 = d["box"]
    # (320-50)/2*? → letterbox 640 空间 xyxy=(270,140,370,340) → 原图 *0.5
    assert abs(x1 - 135) < 1 and abs(y1 - 70) < 1 and abs(x2 - 185) < 1 and abs(y2 - 170) < 1


def test_decode_yolo_class_filtered_out():
    out = np.zeros((1, 84, 8400), dtype=np.float32)
    out[0, 0:4, 0] = [100, 100, 50, 50]
    out[0, 4 + 2, 0] = 0.95  # car
    dets = decode_yolo(out, 0.35, ["person"], 0.45, 640, 640, 1.0, 0, 0)
    assert dets == []


def test_decode_yolo_nms_suppresses_overlap():
    out = np.zeros((1, 84, 8400), dtype=np.float32)
    for i, conf in enumerate([0.9, 0.8]):
        out[0, 0, i] = 100 + i * 3
        out[0, 1, i] = 100
        out[0, 2, i] = 50
        out[0, 3, i] = 50
        out[0, 4, i] = conf
    dets = decode_yolo(out, 0.35, ["person"], 0.45, 640, 640, 1.0, 0, 0)
    assert len(dets) == 1 and abs(dets[0]["conf"] - 0.9) < 1e-6


# ---------- API 契约 ----------

@pytest.fixture()
def client():
    with TestClient(app) as c:
        yield c


H = {"Authorization": "Bearer testtoken"}


def test_healthz_shape(client):
    r = client.get("/v1/healthz", headers=H)
    assert r.status_code == 200
    body = r.json()
    assert body["ready"] is False and body["embedder_ready"] is False


def test_auth_required(client):
    assert client.get("/v1/healthz").status_code == 401
    assert client.get("/v1/healthz", headers={"Authorization": "Bearer wrong"}).status_code == 401


def _jpeg_b64():
    from PIL import Image
    buf = io.BytesIO()
    Image.new("RGB", (64, 64), (10, 200, 30)).save(buf, format="JPEG")
    return base64.b64encode(buf.getvalue()).decode()


def test_judge_409_when_no_model(client):
    r = client.post("/v1/judge", json={"image_b64": _jpeg_b64(), "device_id": "d1"}, headers=H)
    assert r.status_code == 409
    assert r.json()["detail"]["ready"] is False


def test_embed_409_when_no_model(client):
    r = client.post("/v1/embed_face", json={"image_b64": _jpeg_b64()}, headers=H)
    assert r.status_code == 409


def test_judge_422_bad_image(client):
    r = client.post("/v1/judge", json={"image_b64": "a2lkbmQ=", "device_id": "d"},
                    headers=H)  # 合法 base64,非图像
    assert r.status_code == 422


def test_judge_413_too_large(client):
    big = "QUJD" * 2_000_000
    r = client.post("/v1/judge", json={"image_b64": big}, headers=H)
    assert r.status_code == 413
