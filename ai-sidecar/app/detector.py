#!/usr/bin/env python3
"""detector — YOLO11 ONNX 目标检测（输出解码 + 类别过滤 + 类内 NMS）。

契约: ultralytics `yolo export format=onnx` 默认产物, 输出 shape (1, 4+nc, 8400),
前 4 通道为 letterbox(640) 空间的 xywh(中心点), 其余为 COCO 80 类置信度。
"""
from __future__ import annotations

import os

import numpy as np

COCO_CLASSES = [
    "person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck", "boat",
    "traffic light", "fire hydrant", "stop sign", "parking meter", "bench", "bird", "cat",
    "dog", "horse", "sheep", "cow", "elephant", "bear", "zebra", "giraffe", "backpack",
    "umbrella", "handbag", "tie", "suitcase", "frisbee", "skis", "snowboard",
    "sports ball", "kite", "baseball bat", "baseball glove", "skateboard", "surfboard",
    "tennis racket", "bottle", "wine glass", "cup", "fork", "knife", "spoon", "bowl",
    "banana", "apple", "sandwich", "orange", "broccoli", "carrot", "hot dog", "pizza",
    "donut", "cake", "chair", "couch", "potted plant", "bed", "dining table", "toilet",
    "tv", "laptop", "mouse", "remote", "keyboard", "cell phone", "microwave", "oven",
    "toaster", "sink", "refrigerator", "book", "clock", "vase", "scissors",
    "teddy bear", "hair drier", "toothbrush",
]


def decode_yolo(output: np.ndarray, conf_th: float, classes: list[str], iou_th: float,
                img_w: int, img_h: int, scale: float, pad_x: int, pad_y: int):
    """纯函数：(1,84,8400) → [{cls,conf,box(原图px)}]。与 session 无关，可单测。"""
    pred = np.asarray(output[0])  # (84, 8400)
    if pred.shape[0] < 5:
        return []
    pred = pred.T  # (8400, 84)
    scores = pred[:, 4:]
    cls_id = scores.argmax(axis=1)
    conf = scores.max(axis=1)
    keep = conf >= conf_th
    if not keep.any():
        return []
    boxes, conf, cls_id = pred[keep, :4], conf[keep], cls_id[keep]

    # cxcywh → xyxy (letterbox 空间)
    xy = boxes[:, :2]
    wh = boxes[:, 2:]
    xyxy = np.concatenate([xy - wh / 2, xy + wh / 2], axis=1)

    results = []
    want = {c: i for i, c in enumerate(COCO_CLASSES)}
    allowed = [want[c] for c in classes if c in want]
    for cid in sorted(set(cls_id.tolist()) & set(allowed)):
        m = cls_id == cid
        sel = xyxy[m]
        order = conf[m].argsort()[::-1]
        picked = []
        while len(order) > 0:
            i = order[0]
            picked.append(i)
            if len(order) == 1:
                break
            rest = order[1:]
            ious = _iou(sel[i], sel[rest])
            order = rest[ious < iou_th]
        for j in picked:
            x1, y1, x2, y2 = sel[j]
            ox1, oy1 = (x1 - pad_x) / scale, (y1 - pad_y) / scale
            ox2, oy2 = (x2 - pad_x) / scale, (y2 - pad_y) / scale
            clamp = lambda v, lo, hi: float(min(max(v, lo), hi))
            box = [clamp(ox1, 0, img_w), clamp(oy1, 0, img_h), clamp(ox2, 0, img_w), clamp(oy2, 0, img_h)]
            if box[2] - box[0] <= 2 or box[3] - box[1] <= 2:
                continue
            results.append({"cls": COCO_CLASSES[cid], "conf": float(conf[m][j]), "box": box})
    results.sort(key=lambda r: r["conf"], reverse=True)
    return results[:50]


def _iou(box, boxes):
    x1 = np.maximum(box[0], boxes[:, 0])
    y1 = np.maximum(box[1], boxes[:, 1])
    x2 = np.minimum(box[2], boxes[:, 2])
    y2 = np.minimum(box[3], boxes[:, 3])
    inter = np.clip(x2 - x1, 0, None) * np.clip(y2 - y1, 0, None)
    a = (box[2] - box[0]) * (box[3] - box[1])
    b = (boxes[:, 2] - boxes[:, 0]) * (boxes[:, 3] - boxes[:, 1])
    return inter / np.clip(a + b - inter, 1e-9, None)


class YoloDetector:
    def __init__(self, model_path: str) -> None:
        self.ready = False
        self.session = None
        self.input_size = 640
        if model_path and os.path.isfile(model_path):
            import onnxruntime as ort  # 延迟导入: 无模型时不要求装 ort
            so = ort.SessionOptions()
            so.intra_op_num_threads = max(1, min(2, os.cpu_count() or 1))
            self.session = ort.InferenceSession(model_path, sess_options=so, providers=["CPUExecutionProvider"])
            self.input_name = self.session.get_inputs()[0].name
            shape = self.session.get_inputs()[0].shape  # ['batch',3,640,640] 或 [1,3,h,w]
            if isinstance(shape[2], int) and shape[2] > 0:
                self.input_size = shape[2]
            self.ready = True

    def detect(self, img, conf_th: float, classes: list[str], iou_th: float):
        from app.imaging import letterbox
        blob, scale, px, py = letterbox(img, self.input_size)
        out = self.session.run(None, {self.input_name: blob})[0]
        return decode_yolo(out, conf_th, classes, iou_th, img.width, img.height, scale, px, py)
