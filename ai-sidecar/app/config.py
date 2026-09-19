#!/usr/bin/env python3
"""config — novasense-ai sidecar 运行配置（全部环境变量注入，代码零硬编码）。"""
import os


def _int(name: str, default: int) -> int:
    try:
        return int(os.environ.get(name, default))
    except ValueError:
        return default


class Settings:
    def __init__(self) -> None:
        self.token: str = os.environ.get("AI_INTERNAL_TOKEN", "")
        self.model_dir: str = os.environ.get("MODEL_DIR", "/models")
        self.port: int = _int("AI_PORT", 8901)
        self.detect_conf: float = float(os.environ.get("AI_DETECT_CONF", "0.35"))
        self.nms_iou: float = float(os.environ.get("AI_NMS_IOU", "0.45"))
        self.detect_classes: list[str] = [
            c.strip() for c in os.environ.get("AI_DETECT_CLASSES", "person").split(",") if c.strip()
        ]
        self.max_image_bytes: int = _int("AI_MAX_IMAGE_BYTES", 4_500_000)  # base64 上限
        self.backend: str = os.environ.get("AI_BACKEND", "onnxruntime")
        self.detector_file: str = os.environ.get("AI_DETECTOR_FILE", "yolo11n.onnx")
        self.embedder_file: str = os.environ.get("AI_EMBEDDER_FILE", "arcface_r100.onnx")

    @property
    def auth_enabled(self) -> bool:
        return bool(self.token)


settings = Settings()
