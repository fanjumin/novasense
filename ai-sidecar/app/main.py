#!/usr/bin/env python3
"""main — novasense-ai sidecar (FastAPI)。

契约与接口见 docs/AI-UPGRADE-PLAN.md §4。设计纪律：
  · 模型未就绪 → 409 {"ready": false}，Go 侧视为"弃权"(fail-open)。
  · 本服务永不对业务错误抛 5xx；任何非 200 = 弃权。
  · 鉴权: AI_INTERNAL_TOKEN 非空时强制 Bearer；为空 = 仅限受信内网(记 WARNING)。
"""
from __future__ import annotations

import logging
import os
import time
from contextlib import asynccontextmanager

from fastapi import Depends, FastAPI, Header, HTTPException
from pydantic import BaseModel, Field

from app.config import settings
from app.detector import YoloDetector
from app.embedder import FaceEmbedder
from app.imaging import decode_b64

log = logging.getLogger("novasense-ai")
logging.basicConfig(level=logging.INFO, format="[ai] %(asctime)s %(message)s")

state: dict = {"detector": None, "embedder": None, "started": time.time()}


@asynccontextmanager
async def lifespan(_: FastAPI):
    det_path = os.path.join(settings.model_dir, settings.detector_file)
    emb_path = os.path.join(settings.model_dir, settings.embedder_file)
    state["detector"] = YoloDetector(det_path)
    state["embedder"] = FaceEmbedder(emb_path)
    log.info("detector.ready=%s embedder.ready=%s backend=%s",
             state["detector"].ready, state["embedder"].ready, settings.backend)
    if not settings.auth_enabled:
        log.warning("AI_INTERNAL_TOKEN 未设置 —— 内网外暴露 8901 端口即裸奔，生产必须配置 token")
    yield


app = FastAPI(title="novasense-ai", version="0.1.0", lifespan=lifespan)


def auth(authorization: str = Header(default="")):
    if not settings.auth_enabled:
        return
    if authorization != f"Bearer {settings.token}":
        raise HTTPException(status_code=401, detail="bad token")


class JudgeRequest(BaseModel):
    image_b64: str
    device_id: str = ""
    tasks: list[str] = Field(default_factory=lambda: ["detect"])
    classes: list[str] | None = None
    conf: float | None = None


class Detection(BaseModel):
    cls: str
    conf: float
    box: list[float]


class JudgeResponse(BaseModel):
    has_target: bool
    detections: list[Detection]
    top_class: str = ""
    top_conf: float = 0.0
    latency_ms: int = 0


class EmbedRequest(BaseModel):
    image_b64: str


class EmbedResponse(BaseModel):
    embedding: list[float]
    quality: float
    dim: int


@app.get("/v1/healthz")
def healthz(_: None = Depends(auth)):
    det, emb = state["detector"], state["embedder"]
    return {
        "ready": bool(det and det.ready),
        "embedder_ready": bool(emb and emb.ready),
        "backend": settings.backend,
        "classes_watchlist": settings.detect_classes,
        "uptime_s": int(time.time() - state["started"]),
    }


@app.post("/v1/judge", response_model=JudgeResponse)
def judge(req: JudgeRequest, _: None = Depends(auth)):
    det = state["detector"]
    if len(req.image_b64) > settings.max_image_bytes:
        raise HTTPException(status_code=413, detail="image too large")
    try:
        img = decode_b64(req.image_b64)
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    if det is None or not det.ready:
        raise HTTPException(status_code=409, detail={"ready": False, "reason": "detector not loaded"})

    t0 = time.perf_counter()
    classes = req.classes or settings.detect_classes
    conf_th = req.conf if req.conf is not None else settings.detect_conf
    dets = det.detect(img, conf_th, classes, settings.nms_iou)
    latency = int((time.perf_counter() - t0) * 1000)

    detections = [Detection(**d) for d in dets]
    top = detections[0] if detections else None
    return JudgeResponse(
        has_target=bool(detections),
        detections=detections,
        top_class=top.cls if top else "",
        top_conf=top.conf if top else 0.0,
        latency_ms=latency,
    )


@app.post("/v1/embed_face", response_model=EmbedResponse)
def embed_face(req: EmbedRequest, _: None = Depends(auth)):
    emb = state["embedder"]
    if len(req.image_b64) > settings.max_image_bytes:
        raise HTTPException(status_code=413, detail="image too large")
    try:
        img = decode_b64(req.image_b64)
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    if emb is None or not emb.ready:
        raise HTTPException(status_code=409, detail={"ready": False, "reason": "embedder not loaded"})
    result = emb.embed(img)
    if result is None:
        raise HTTPException(status_code=422, detail="degenerate face crop")
    vec, quality = result
    return EmbedResponse(embedding=vec, quality=quality, dim=len(vec))


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="127.0.0.1", port=settings.port)
