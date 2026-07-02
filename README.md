# 视频流管理平台 v0.8.1

一站式摄像头监控管理平台：拉流 → 转码 → 分发 → 导播 → 录制 → 抓拍 → 云台控制 → 人脸识别 → 告警通知 → 双向对讲。

支持标准 RTSP/ONVIF 摄像头、旧手机改摄像头、USB 摄像头统一接入。

---

## 目录

- [架构](#架构)
- [功能清单](#功能清单)
- [快速开始](#快速开始)
  - [本地运行](#本地运行)
  - [Docker 部署](#docker-部署)
- [配置](#配置)
- [API 文档](#api-文档)
- [前端界面](#前端界面)
- [开发指南](#开发指南)
- [常见问题](#常见问题)

---

## 架构

```
┌──────────────────────────────────────────────────────────────────┐
│                         Go 后端 ( :8899 )                        │
│  ┌──────────┐ ┌────────────┐ ┌─────────┐ ┌───────┐ ┌─────────┐ │
│  │ 设备管理  │ │ FFmpeg管理 │ │ 录制/   │ │ 导播  │ │ 人脸识别 │ │
│  │ 增删改查  │ │ 拉流+推流  │ │ 定时/抓拍│ │ 切换  │ │ + 匹配  │ │
│  └──────────┘ └────────────┘ └─────────┘ └───────┘ └─────────┘ │
│        │              │                            │            │
│  ┌─────┴──────┐  ┌────┴──────┐            ┌────────┴───────┐  │
│  │ SQLite DB  │  │ FFmpeg    │            │  WebSocket SSE │  │
│  │ store.db   │  │ (多进程)  │──RTMP──→   │  事件推送      │  │
│  └────────────┘  └───────────┘            └────────────────┘  │
├──────────────────────────────────────────────────────────────────┤
│              MediaMTX ( :8888 / :1935 / :8554 / :8889 )          │
│   RTSP ← 摄像头拉流  │  RTMP ← FFmpeg推流  │  HLS / WebRTC      │
├──────────────────────────────────────────────────────────────────┤
│                  Web 前端 ( /ui/index.html )                      │
│  监控 │ 录像 │ 定时 │ 抓拍 │ 导播 │ 运动 │ 人脸 │ 发现 │ 状态    │
└──────────────────────────────────────────────────────────────────┘
```

### 组件

| 组件 | 角色 | 端口 |
|------|------|------|
| Go 后端 | 核心服务：API + 设备管理 + FFmpeg 调度 + 事件推送 | 8899 |
| MediaMTX | 流媒体服务器：RTSP/RTMP/HLS/WebRTC 分发 | 8888(HLS) 8554(RTSP) 1935(RTMP) 8889(WEBRTC) |
| FFmpeg | 拉流代理 + 转码 + 录制 + 运动检测 | - |
| SQLite | 持久化存储：设备、录像、配置、人脸 | data/store.db |

---

## 功能清单

### 🎥 监控

- ✅ 添加/删除摄像头（RTSP / HTTP MJPEG / USB）
- ✅ 一键启停 FFmpeg 代理
- ✅ HLS 实时播放（hls.js，延迟 3-5s）
- ✅ **WebRTC 低延迟播放**（WHEP 协议，延迟 <1s，自动回退 HLS）
- ✅ 分辨率切换（480p / 720p / 1080p / 2K / 4K）
- ✅ 音量开关 / 全屏播放
- ✅ 自动重连（代理崩溃后 3s 恢复）
- ✅ 设备卡片拖拽排序（localStorage 持久化）

### 🎮 云台控制（PTZ）

- ✅ 十字方向键：按住移动，松开停止
- ✅ 变焦 ＋/－
- ✅ 速度滑块（1~10 级）
- ✅ 预置位 P1/P2/P3（一键跳转）
- ✅ 支持协议：海康 ISAPI、旧版 CGI

### 🎬 导播台

- ✅ 多路画面同屏预览
- ✅ 点击子画面切换 PGM 主输出
- ✅ 黑场/测试图信号源
- ✅ FFmpeg 合成输出
- ✅ RTMP 推流到第三方平台（B站/抖音/YouTube 等）

### 🔴 推流

- ✅ 支持推送至抖音、快手、B 站、YouTube 等 RTMP 平台
- ✅ 可配置分辨率、码率、帧率
- ✅ 独立音频编码（AAC 128kbps）

### 💾 录制

- ✅ 手动录制指定时长
- ✅ 定时录制计划（按时间段 + 星期循环）
- ✅ 运动触发自动录制
- ✅ 视频文件在线播放 / 下载
- ✅ 自动分段录制

### 📸 抓拍

- ✅ 定时抓拍（可配间隔）
- ✅ 抓拍历史浏览
- ✅ 保留天数设置

### 📡 设备发现

- ✅ ARP 扫描（局域网设备发现）
- ✅ ONVIF 探测（标准摄像头自动发现）
- ✅ SSDP 扫描
- ✅ 端口扫描 + HTTP 指纹识别
- ✅ RTSP 路径猜测
- ✅ 设备类型分类（摄像头/路由器/NAS/打印机等）
- ✅ 批量导入发现设备

### 🏃 运动检测

- ✅ FFmpeg scene 检测（灵敏度可调 0.1-0.9）
- ✅ 冷却时间设置
- ✅ 运动触发录像
- ✅ 运动时间轴（缩略图列表）
- ✅ **SSE 实时推送 + 浏览器通知**

### 👤 人脸识别

- ✅ **pigo 纯 Go 人脸检测**（无 CGo 依赖）
- ✅ Haar Cascade 分类器
- ✅ 自动裁剪人脸缩略图
- ✅ 感知哈希（aHash）匹配
- ✅ 已知人脸 vs 陌生人自动区分
- ✅ 用户可标记未知人脸姓名
- ✅ 识别事件记录 + 置信度显示

### 🏷️ 设备分组

- ✅ 按房间/楼层/类型分组
- ✅ 分组过滤筛选
- ✅ 分组管理（新建/重命名/删除）
- ✅ 设备卡片显示分组标签

### 🎤 双向对讲

- ✅ **WebSocket 实时音频传输**
- ✅ 浏览器麦克风采集（getUserMedia）
- ✅ PCM → AAC → RTMP 推流到 MediaMTX
- ✅ 按住说话（Push-to-Talk）
- ✅ 摄像头端通过 RTSP 拉取音频播放

### 🔔 本地告警

- ✅ **Server-Sent Events (SSE)** 实时推送
- ✅ 浏览器 Web Notification API
- ✅ 点击通知切到监控页
- ✅ 8 秒自动关闭

### 📊 系统状态

- ✅ 设备总数 / 在线数 / 录像数
- ✅ 磁盘使用率进度条
- ✅ **存储管理**：保留天数 / 最大磁盘 % / 自动清理 / 手动清理

### 🔐 授权管理

- ✅ 登录认证（默认密码 admin）
- ✅ 授权码生成/校验/绑定
- ✅ 免费版 / Pro 版区分

### 🐳 Docker 部署

- ✅ Docker Compose 一键启动
- ✅ 数据持久化

---

## 快速开始

### 本地运行

#### 前提

- Go 1.25+
- FFmpeg 7.0+（需支持 libx264、aac）
- MediaMTX（Docker 或直接安装）

#### 启动步骤

```bash
# 1. 启动 MediaMTX（Docker）
docker compose up -d mediamtx

# 2. 编译后端
cd backend
go build -o ../video-stream-manager .

# 3. 启动后端
cd ..
DATA_DIR=./data ./video-stream-manager

# 4. 打开浏览器
# http://localhost:8899
# 默认密码: admin
```

### Docker 部署

```bash
# 一键启动（含 MediaMTX + 后端）
docker compose up -d

# 查看日志
docker compose logs -f

# 访问
# http://localhost:8899
```

---

## 配置

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `ADMIN_PASSWORD` | `admin` | Web 管理密码 |
| `DATA_DIR` | `/app/data` | 数据目录（数据库、录像、抓拍） |
| `VPS_PLUGIN_URL` | - | easykai.cn 插件地址（远程功能） |
| `VPS_API_KEY` | - | easykai.cn API Key |

### 存储设置

通过 Web UI `录像 → ⚙️ 存储` 配置：

| 设置项 | 默认值 | 说明 |
|--------|--------|------|
| 保留天数 | 30 | 超过此天数的录像自动删除 |
| 最大磁盘使用率 | 80% | 磁盘使用率超过此值触发清理 |
| 自动清理 | 开启 | 每 6 小时检查一次 |

### 运动检测

通过 Web UI `运动` Tab 配置：

| 设置项 | 默认值 | 说明 |
|--------|--------|------|
| 灵敏度 | 0.3 | 值越低越敏感（0.1-0.9） |
| 冷却时间 | 30s | 两次检测之间最短间隔 |

---

## API 文档

所有 API 均需认证（`/api/login` 获取 session cookie），除 `/api/health`、`/api/events`、`/api/license/check` 外。

### 设备管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/devices` | 设备列表 |
| POST | `/api/devices` | 添加设备 |
| GET | `/api/devices/{id}` | 设备详情 |
| PUT | `/api/devices/{id}` | 更新设备（分组/名称） |
| DELETE | `/api/devices/{id}` | 删除设备 |
| GET | `/api/groups` | 分组列表（含设备数） |
| PUT | `/api/groups` | 重命名分组 |

### 流代理

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/proxy/start/{id}` | 启动代理 |
| POST | `/api/proxy/stop/{id}` | 停止代理 |
| POST | `/api/proxy/config/{id}` | 修改分辨率 |

### 录制

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/recordings` | 录像列表（`?device_id=` 筛选） |
| POST | `/api/recordings` | 手动录制 |
| DELETE | `/api/recordings?id=` | 删除录像 |

### 定时计划

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/schedules` | 计划列表 |
| POST | `/api/schedules` | 添加计划 |
| GET | `/api/schedules/{id}` | 计划详情 |
| PUT | `/api/schedules/{id}` | 更新计划 |
| DELETE | `/api/schedules/{id}` | 删除计划 |

### 抓拍

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/snapshots` | 抓拍配置列表 |
| POST | `/api/snapshots` | 开启抓拍 |
| PUT | `/api/snapshots/{id}` | 更新配置 |
| DELETE | `/api/snapshots/{id}` | 关闭 |
| GET | `/api/snapshots/list/{id}` | 抓拍历史列表 |

### 运动检测

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/motion/config` | 所有设备运动配置 |
| GET | `/api/motion/config/{id}` | 单设备配置 |
| POST | `/api/motion/config/{id}` | 设置配置 |
| GET | `/api/motion/events/{id}` | 事件列表（`?limit=`） |
| GET | `/api/motion/events/all` | 全部事件 |

### 人脸识别

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/faces` | 已知人脸列表 |
| POST | `/api/faces` | 手动添加人脸 |
| GET | `/api/faces/{id}` | 人脸详情 |
| PUT | `/api/faces/{id}` | 标记姓名 |
| DELETE | `/api/faces/{id}` | 删除人脸 |
| GET | `/api/face-events` | 识别事件列表（`?device_id=`） |
| GET | `/api/face-events/{motion_id}` | 某运动事件的人脸识别结果 |

### 导播

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/director` | 导播状态 |
| GET | `/api/director/sources` | 信号源列表 |
| POST | `/api/director/switch` | 切换 PGM |
| POST | `/api/director/stop` | 停止 PGM |
| POST | `/api/director/push` | 设置推流地址 |
| POST | `/api/director/overlays` | 叠加层配置 |

### 云台控制

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/ptz/{id}/{action}` | 云台动作（up/down/left/right/zoom_in/zoom_out） |
| POST | `/api/ptz/{id}/stop` | 停止 |
| POST | `/api/ptz/{id}/preset/{n}` | 跳转预置位 |

### 存储管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/storage` | 存储状态 + 配置 |
| PUT | `/api/storage` | 更新保留配置 |
| POST | `/api/storage` | 手动触发清理 |

### 实时事件

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/events` | **SSE** 实时事件流（运动检测推送） |

### 双向对讲

| 方法 | 路径 | 说明 |
|------|------|------|
| WebSocket | `/api/talk/{id}` | 浏览器 → PCM → FFmpeg → RTMP 推流 |

### 设备发现

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/discover/start` | 开始扫描 |
| POST | `/api/discover/stop` | 停止扫描 |
| GET | `/api/discover/status` | 扫描状态 + 结果 |
| POST | `/api/discover/import` | 导入选中设备 |

### 系统

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/status` | 系统状态（含磁盘信息） |
| GET | `/api/health` | 健康检查 |
| POST | `/api/login` | 登录 |
| POST | `/api/logout` | 退出 |

### 授权

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/license/generate` | 生成授权码 |
| POST | `/api/license/check` | 校验授权码 |
| POST | `/api/license/bind` | 绑定设备 |
| GET | `/api/license/list` | 授权码列表 |

---

## 前端界面

### Tab 说明

| Tab | 功能 |
|-----|------|
| 📺 **监控** | 主界面：设备卡片网格、观看、PTZ、分组筛选、拖拽排序 |
| 💾 **录像** | 录制历史、日期筛选、设备搜索、存储管理、删除 |
| ⏰ **定时** | 定时录制计划管理 |
| 📸 **抓拍** | 抓拍配置、历史浏览 |
| 🎬 **导播** | 多路导播、PGM 切换、RTMP 推流 |
| 🏃 **运动** | 运动检测配置、事件时间轴 |
| 👤 **人脸** | 已知人脸库、命名、识别记录 |
| 📡 **发现** | 局域网设备扫描、批量导入 |
| 📊 **状态** | 系统信息、版本、磁盘使用率 |

### 支持的浏览器

- Chrome 80+
- Firefox 80+
- Edge 80+
- Safari 14+（WebRTC 支持有限）

---

## 开发指南

### 项目结构

```
video-stream-manager/
├── backend/                # Go 后端源码
│   ├── main.go            # 主程序（API、FFmpeg 管理、事件）
│   ├── store.go           # SQLite 数据层
│   ├── face_detector.go   # 人脸检测（pigo）
│   ├── director.go        # 导播台逻辑
│   ├── discovery.go       # 设备发现逻辑
│   ├── facefinder         # Haar Cascade 分类器文件
│   └── go.mod             # Go 模块定义
├── frontend/              # Web 前端
│   ├── index.html         # 单页应用（~80KB）
│   └── login.html         # 登录页
├── mediamtx/              # MediaMTX 配置
│   └── mediamtx.yml       # 流媒体服务器配置
├── deploy/                # 部署文件
│   ├── setup.sh           # 安装脚本
│   ├── mediamtx.service   # systemd 服务
│   └── video-stream-manager.service
├── data/                  # 数据目录（运行时）
│   ├── store.db           # SQLite 数据库
│   ├── videos/            # 录像文件
│   ├── snapshots/         # 抓拍/运动截图
│   └── faces/             # 人脸缩略图
├── Dockerfile             # 后端 Docker 镜像
├── docker-compose.yml     # Docker Compose 编排
├── PLAN.md                # 产品规划文档
├── SOLUTION.md            # 技术方案
└── README.md              # 本文件
```

### 构建

```bash
# 开发构建
cd backend
go build -o ../video-stream-manager .

# 带版本信息
go build -ldflags="-s -w -X main.version=0.8.1" -o ../video-stream-manager .
```

### 添加新功能

1. **后端 API**：在 `main.go` 的 `registerRoutes()` 添加路由，实现 handler
2. **数据层**：在 `store.go` 的 `migrate()` 添加 DDL，实现 CRUD 方法
3. **前端**：在 `index.html` 添加 Tab/Modal/JS 逻辑
4. **版本更新**：修改所有 `"0.8.0"` → `"0.8.x"`，`git commit`

---

## 常见问题

### Q: 摄像头画面黑屏？

- 确认 RTSP 地址可访问：`ffplay rtsp://user:pass@ip:554/stream`
- 检查 FFmpeg 是否安装：`ffmpeg -version`
- 检查 MediaMTX 是否运行：`curl http://localhost:8888/`

### Q: WebRTC 不工作？

- WebRTC 仅局域网有效（无 TURN 服务器）
- 浏览器需要支持 WebRTC
- 自动降级到 HLS，不影响观看

### Q: 人脸检测没反应？

- 运动检测必须先开启
- 人脸需要正面朝向摄像头
- 最小人脸尺寸 40×40 像素
- 查看后端日志：`[face] detected N face(s)`

### Q: 磁盘空间不足？

- 设置合理保留天数
- 开启自动清理
- 手动清理：`录像 → ⚙️ 存储 → 立即清理`

### Q: 如何远程访问？

- 当前版本远程功能未开放
- 未来支持 Tailscale/FRP/Cloudflare Tunnel

---

## License

MIT License

Copyright (c) 2026 NetCam Center
