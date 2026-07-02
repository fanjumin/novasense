# 本地功能开发计划（6 阶段）

## 总体原则

- **纯本地运行**，不依赖 VPS/远程服务
- 所有改动先出本文件，确认后逐阶段实施
- 每个阶段独立可交付，不阻塞后续阶段

---

## 阶段 1：人脸识别

### 目标

移动检测触发抓拍后，对截图做人脸检测 + 基础识别，标注已知人/陌生人，存入 DB，前端展示。

### 方案

使用系统已安装的 OpenCV 4.5.4 dev 库 + Go 绑定（`gocv.io/x/gocv`）做：

1. **人脸检测** — Haar Cascade 分类器找画面中的人脸区域
2. **人脸裁剪 + 特征编码** — 将人脸区域缩放到统一尺寸，计算感知哈希（pHash），作为人脸指纹
3. **人脸匹配** — 新抓拍的人脸指纹 vs DB 中已知人脸指纹（汉明距离 < 阈值 → 匹配）
4. **存储** — 新增 `faces` 表（device_id, face_hash, label, thumbnail_path, created_at）和 `face_events` 表（关联 motion_event + 检测到的人脸）
5. **触发时机** — 在 motion detector 捕获 snapshot 之后、VPS AI 分析之前插入

### 后端改动

| 文件 | 改动 |
|------|------|
| `backend/go.mod` | 加 `gocv.io/x/gocv` |
| `backend/face_detector.go` | 新文件：人脸检测 + 编码 + 匹配逻辑 |
| `backend/store.go` | 新增 faces / face_events 表 + CRUD |
| `backend/main.go` | motion 检测后调 face_detector，新增 `/api/faces/`、`/api/face-events/` API |

### 前端改动

`frontend/index.html`:
- 新增 "人脸" Tab（列表显示已知人脸 + 最近识别事件）
- 运动事件详情页显示检测到的人脸缩略图 + 标签
- 支持用户为未知人脸命名（标记为"张三"）

### 已知限制

- 仅 Haar Cascade — 无法区分双胞胎，侧脸/遮挡识别率低
- pHash 匹配适合同角度/同光照，大变脸会误判
- **不依赖 GPU**，单张 720p 截图处理 < 200ms

---

## 阶段 2：录像存储管理

### 目标

- 磁盘空间监控（总/已用/可用，录像目录单独统计）
- 自动清理策略：按保留天数 / 按最大磁盘使用率 / 按录像个数
- 录像回放前端增强：时间轴 / 按日期筛选 / 搜索

### 方案

| 功能 | 实现 |
|------|------|
| 磁盘统计 | Go `syscall.Statfs` → 前端 `/api/status` 返回 |
| 清理策略 | 新增 `retention_settings` 表 + 后台 goroutine 定时清理 |
| 前端回放 | 时间轴组件（按天分组，彩色条显示活跃时段） |

### 后端改动

| 文件 | 改动 |
|------|------|
| `backend/store.go` | 新增 `retention_settings` 表 + streaming_settings CRUD |
| `backend/main.go` | status 返回磁盘信息；后台启动清理 goroutine；新增 `/api/storage` 配置 API |

### 前端改动

- 新增 "存储" 设置面板（保留天数/最大使用率）
- 录像页面增加时间轴、日期筛选、搜索框
- 单条录像显示文件大小

---

## 阶段 3：WebRTC 低延迟播放（局域网）

### 目标

局域网内 HLS 延迟 3-5s → WebRTC < 1s

### 方案

MediaMTX 已配好 WebRTC（端口 8889/8554），前端只需：
1. MediaMTX 提供 WebRTC URL: `webrtc://{gateway_ip}:8889/live/{device_id}`
2. 前端使用 `wrtc` 或 MediaMTX 自带的 `webrtc-ws` 协议通过 WHIP 拉流
3. 播放器降级：WebRTC → HLS（WebRTC 失败自动回退）

### 前端改动

`frontend/index.html`:
- 新增 WebRTC 播放器（基于 WebRTC API）
- 自动检测浏览器 WebRTC 支持
- 画质切换保留（通过 `/api/proxy/config/` 改分辨率，WebRTC 自动适配）

### 后端改动

- 无（MediaMTX 已经处理 WebRTC，只需把正确的 URL 通过设备状态 API 返回）

### 前提

当前 `mediamtx.yml` 需要确认 WebRTC 配置正确。

---

## 阶段 4：设备分组/标签

### 目标

按房间/楼层/类型组织设备，方便管理。

### 后端改动

`Device` 结构已含 `GroupName` 字段，只需：
- 新增 `/api/groups` API（列出所有分组名 + 设备计数）
- 前端批量移动设备到分组
- 分组颜色/图标（可选）

### 前端改动

- 监控页面增加分组下拉过滤（"全部 / 客厅 / 卧室 / 厨房 / 门外"）
- 设备卡片显示分组标签
- 编辑设备时可选分组
- 分组管理页面（新增/重命名/删除分组）

---

## 阶段 5：本地告警通知

### 目标

移动检测触发时，浏览器弹系统通知（不依赖外部推送）。

### 方案

Web Notification API：
1. 前端请求 `Notification.permission`（用户点击允许）
2. 后端新增 Server-Sent Events（SSE）端点 `/api/events`
3. 移动检测触发时，后端推送 SSE 事件
4. 前端收到事件后 `new Notification(...)` 弹窗
5. 点击通知跳转到对应设备的画面

### 后端改动

| 文件 | 改动 |
|------|------|
| `backend/main.go` | 新增 SSE handler `/api/events`；motion检测后向所有SSE客户端广播事件 |

### 前端改动

- 加载时请求通知权限
- 连接 SSE，收到 motion 事件后弹系统通知
- 点击通知 → 跳转到该设备全屏画面

---

## 阶段 6：双向音频对讲（局域网）

### 目标

在观看端采集麦克风 → 推送音频到摄像头端播放。

### 方案

| 端 | 实现 |
|------|------|
| 观看端（浏览器） | `getUserMedia` 采集麦克风 → WebRTC DataChannel / WebSocket 发送 PCM/AAC |
| Go 后端 | 接收音频流 → 通过 RTSP 推送到摄像头（或通过 FFmpeg 推音频到 MediaMTX） |
| 摄像头端（netcam-android） | RTSP 客户端拉取音频流 → AudioTrack 播放 |

### 注意

- 浏览器 WebRTC 采集 → 后端需要转发模块
- netcam-android 已有 AAC 播放能力但需要调通 RTSP 音频拉流
- 本阶段只需完成后端转发 + 浏览器采集部分，手机端后续适配

### 后端改动

| 文件 | 改动 |
|------|------|
| `backend/main.go` | 新增 WebSocket `/api/talk/{device_id}` — 接收浏览器音频数据，推送到 MediaMTX 音频 channel |

### 前端改动

- 设备卡片增加"对讲"按钮
- 点击后请求麦克风权限
- 音频实时发送到后端

---

## 依赖检查（初始）

| 依赖 | 状态 | 操作 |
|------|------|------|
| OpenCV 4.5.4 dev | ✅ 已装 | 直接使用 |
| Go | ✅ | 已有 |
| gocv | ❌ | `go get gocv.io/x/gocv` |
| 系统 libopencv-dev | ✅ | 已装 |
| FFmpeg | ✅ | 已有 |
| MediaMTX | ✅ | Docker 已有 |

---

## 实施顺序

```
Phase 1: 人脸识别 ─────────────────────── 当前
Phase 2: 录像存储管理 ──── ⬆️ 依赖 Phase 1 完成
Phase 3: WebRTC ─────────── ⬆️
Phase 4: 设备分组 ────────── ⬆️
Phase 5: 本地通知 ────────── ⬆️
Phase 6: 双向对讲 ────────── ⬆️
```

确认后从 Phase 1 开始写代码。
