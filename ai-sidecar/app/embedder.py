#!/usr/bin/env python3
"""embedder — 人脸嵌入向量 (InsightFace 系 ONNX, 112x112 输入)。

v0.1 简化: 用 pigo bbox 裁剪后直接 resize(不做五点仿射对齐),
对光照鲁棒性已远优于 aHash; 后续版本可接入 alignment 提升跨姿态精度。
输出: 512 维 L2 归一化 float32 数组(与 Go 侧 embedding BLOB 的小端 float32 编码对应)。
"""
from __future__ import annotations

import os

import numpy as np
from PIL import Image


class FaceEmbedder:
    def __init__(self, model_path: str) -> None:
        self.ready = False
        self.session = None
        self.input_size = 112
        if model_path and os.path.isfile(model_path):
            import onnxruntime as ort
            self.session = ort.InferenceSession(model_path, providers=["CPUExecutionProvider"])
            self.input_name = self.session.get_inputs()[0].name
            self.ready = True

    def embed(self, img: Image.Image):
        blob = self._preprocess(img)
        vec = np.asarray(self.session.run(None, {self.input_name: blob})[0]).reshape(-1)
        norm = float(np.linalg.norm(vec))
        if norm < 1e-9:
            return None
        emb = (vec / norm).astype(np.float32)
        quality = self._estimate_quality(img)
        return emb.tolist(), quality

    def _preprocess(self, img: Image.Image) -> np.ndarray:
        img = img.convert("RGB").resize((self.input_size, self.input_size), Image.BILINEAR)
        arr = np.asarray(img, dtype=np.float32)          # RGB, 0-255
        arr = (arr - 127.5) / 127.5
        return np.ascontiguousarray(arr.transpose(2, 0, 1)[None])

    @staticmethod
    def _estimate_quality(img: Image.Image) -> float:
        """粗糙质量分: 分辨率 + 对比度启发式, 供 Go 端低质量脸丢弃用。"""
        w, h = img.size
        res = min(1.0, min(w, h) / 112.0)
        arr = np.asarray(img.convert("L"), dtype=np.float32)
        contrast = min(1.0, float(arr.std()) / 40.0)
        return round(0.5 * res + 0.5 * contrast, 3)
