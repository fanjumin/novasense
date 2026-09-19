#!/usr/bin/env python3
"""imaging — 图像解码与 YOLO letterbox 预处理（纯 PIL+numpy，无 OpenCV 依赖）。"""
import base64

import numpy as np
from PIL import Image

LETTERBOX_PAD = 114  # ultralytics 默认灰边


def decode_b64(image_b64: str) -> Image.Image:
    """base64(可含 dataURL 前缀) → PIL RGB。异常直接抛 ValueError。"""
    if "," in image_b64[:64] and image_b64.strip().lower().startswith("data:"):
        image_b64 = image_b64.split(",", 1)[1]
    try:
        raw = base64.b64decode(image_b64, validate=False)
    except Exception as exc:  # binascii 系列
        raise ValueError(f"invalid base64: {exc}") from exc
    from io import BytesIO
    try:
        img = Image.open(BytesIO(raw)).convert("RGB")
    except Exception as exc:
        raise ValueError(f"cannot decode image: {exc}") from exc
    if img.width < 16 or img.height < 16:
        raise ValueError("image too small")
    return img


def letterbox(img: Image.Image, size: int = 640):
    """等比缩放+补边 → (blob CHW float32/255, scale, pad_x, pad_y)。"""
    w0, h0 = img.size
    scale = min(size / w0, size / h0)
    nw, nh = max(1, round(w0 * scale)), max(1, round(h0 * scale))
    resized = img.resize((nw, nh), Image.BILINEAR)
    canvas = Image.new("RGB", (size, size), (LETTERBOX_PAD, LETTERBOX_PAD, LETTERBOX_PAD))
    pad_x, pad_y = (size - nw) // 2, (size - nh) // 2
    canvas.paste(resized, (pad_x, pad_y))
    arr = np.asarray(canvas, dtype=np.float32).transpose(2, 0, 1)[None] / 255.0
    return np.ascontiguousarray(arr), scale, pad_x, pad_y
