# 视频流管理平台 v0.3.0

一站式摄像头监控管理平台：拉流 → 转码 → 分发（HLS/RTMP/WebRTC）→ 导播 → 录制 → 抓拍 → 云台控制。

## 架构

```
┌──────────────────────────────────────────────────────────────┐
│                    Go 后端 ( :8899 )                         │
│  ┌──────────┐ ┌────────────┐ ┌─────────┐ ┌───────────────┐  │
│  │ 设备管理  │ │ FFmpeg管理 │ │ 录制/    │ │ PTZ 云台控制  │  │
│  │ 增删改查  │ │ 拉流+推流  │ │ 定时/抓拍│ │ ISAPI/CGI    │  │
│  └──────────┘ └────────────┘ └─────────┘ └───────────────┘  │
│        │              │                                      │
│  ┌─────┴──────┐  ┌────┴──────┐                               │
│  │ SQLite DB  │  │ FFmpeg    │  ──RTMP──→                    │
│  │ store.db   │  │ (多进程)  │                                │
│  └────────────┘  └───────────┘                               │
├──────────────────────────────────────────────────────────────┤
│                  MediaMTX ( :8888 / :1935 / :8554 )          │
│  RTSP ← 摄像头拉流  │  RTMP ← FFmpeg推流  │  HLS / WebRTC    │
├──────────────────────────────────────────────────────────────┤
│                 Web 前端 ( /ui/ )                            │
│  监控 │ 推流 │ 录像 │ 定时 │ 抓拍 │ 导播 │ 状态             │
└──────────────────────────────────────────────────────────────┘
```

## 功能

### 🎥 监控
- 添加/删除摄像头（RTSP 协议）
- 一键"观看"自动启停 FFmpeg 代理
- 每路摄像头独立 HLS 流（1280×720, 15fps, 2000kbps）
- 自动重新连接（代理崩溃后 3s 恢复）
- 🔇 音量开关
- ⛶ 视频全屏

### 🎮 云台控制（PTZ）
- 十字方向键：按住移动，松开停止
- 变焦 ＋/－
- 速度滑块（1~10 级）
- 预置位 P1/P2/P3（一键跳转）
- 支持协议：海康 ISAPI（`/ISAPI/PTZCtrl/channels/1/continuous`）、旧版 CGI（`/cgi-bin/ptz.cgi`）

### 🎬 导播台
- 多路画面同屏显示（1 主屏 + N 子画面）
- 点击子画面切换主屏
- 自动启停所有摄像头代理

### 🔴 推流
- 支持推送至抖音、快手、B 站、YouTube 等 RTMP 平台
- 可配置分辨率、码率、帧率
- 独立音频编码（AAC 128kbps）

### 💾 录制
- 手动录制指定时长
- 定时录制计划（按时间段 + 星期循环）
- 视频文件管理（在线播放/下载）
- 自动分段录制

### 📸 抓拍
- 定时截图（可配置间隔）
- 历史抓拍画廊
- 自动清理旧快照（可配置保留天数）

### 🔊 音频
- FFmpeg 代理默认音频编码（AAC 64kbps, 16000Hz, 单声道）
- 推流任务音频（AAC 128kbps, 44100Hz, 立体声）
- 摄像头 RTSP 流含音频时自动推送

### 📊 状态面板
- 设备在线数、任务数、录像数概览
- 系统实时状态 JSON

## 快速开始

### 依赖

| 组件 | 版本 | 用途 |
|------|------|------|
| Go | ≥1.20 | 后端服务器 |
| FFmpeg | ≥4.4 | 拉流转码推流 |
| MediaMTX | ≥1.9 | 流媒体网关 |

```bash
# 检查依赖
ffmpeg -version
mediamtx --version
go version
```

### 启动

```bash
cd ~/projects/video-stream-manager

# 1. 启动流媒体网关
mediamtx mediamtx/mediamtx.yml &

# 2. 启动后端
./bin/video-stream-manager &

# 3. 打开浏览器
# http://localhost:8899
```

### 编译

```bash
cd backend && go build -o ../bin/video-stream-manager .
```

## API 文档

### 设备管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/devices` | 设备列表 |
| POST | `/api/devices` | 添加设备 |
| DELETE | `/api/devices/{id}` | 删除设备 |
| GET | `/api/status` | 系统状态 |

### 直播代理

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/proxy/start/{id}` | 启动拉流代理 |
| POST | `/api/proxy/stop/{id}` | 停止拉流代理 |

### 云台控制

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/ptz/{id}/up` | 上移 |
| POST | `/api/ptz/{id}/down` | 下移 |
| POST | `/api/ptz/{id}/left` | 左移 |
| POST | `/api/ptz/{id}/right` | 右移 |
| POST | `/api/ptz/{id}/zoom_in` | 放大 |
| POST | `/api/ptz/{id}/zoom_out` | 缩小 |
| POST | `/api/ptz/{id}/stop` | 停止 |
| POST | `/api/ptz/{id}/preset/{n}` | 跳转到预置位 n |
| | `?speed=1~10` | 控制速度 |

### 推流

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/push-tasks` | 推流任务列表 |
| POST | `/api/push-tasks` | 创建推流任务 |
| POST | `/api/push-tasks/{id}/start` | 启动推流 |
| POST | `/api/push-tasks/{id}/stop` | 停止推流 |
| DELETE | `/api/push-tasks/{id}` | 删除推流任务 |

### 录制与抓拍

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/recordings` | 录像列表 |
| POST | `/api/recordings` | 手动录制 |
| GET/POST/PUT/DELETE | `/api/schedules` | 定时计划 CRUD |
| GET/POST/PUT/DELETE | `/api/snapshots` | 抓拍配置 CRUD |
| GET | `/api/snapshots/list/{id}` | 设备抓拍历史 |

### 系统

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 健康检查 |
| GET | `/api/export` | 导出所有配置 |
| POST | `/api/import` | 导入配置 |

## 配置

### 数据存储

- SQLite: `data/store.db`
- 录像: `data/videos/{device_id}/{date}/{time}.mp4`
- 抓拍: `data/snapshots/{config_id}/{timestamp}.jpg`

### MediaMTX 配置

见 `mediamtx/mediamtx.yml`，主要参数：

| 参数 | 值 | 说明 |
|------|-----|------|
| hlsAddress | :8888 | HLS 播放端口 |
| rtmpAddress | :1935 | RTMP 接收端口 |
| rtspAddress | :8554 | RTSP 服务端口 |
| hlsAlwaysRemux | true | 持续生成 HLS |
| hlsVariant | lowLatency | 低延迟模式 |

FFmpeg 通过 RTMP 推流至 MediaMTX，路径格式：
```
rtmp://127.0.0.1:1935/live/{device_id}
```
HLS 播放地址：
```
http://{host}:8888/live/{device_id}/
```

## 运行管理

### 进程管理

```bash
# 查看进程
ps aux | grep -E 'video-stream-manager|mediamtx|ffmpeg'

# 停止后端
kill $(pgrep -f 'video-stream-manager')

# 重启流程
kill $(pgrep -f 'video-stream-manager')
kill $(pgrep -f mediamtx)
mediamtx mediamtx/mediamtx.yml &
./bin/video-stream-manager &
```

### MediaMTX 常见问题

MediaMTX 在 RTMP 流中断时可能崩溃。建议使用 `systemd` 自动重启：

```ini
# /etc/systemd/system/mediamtx.service
[Unit]
Description=MediaMTX
After=network.target

[Service]
ExecStart=/usr/local/bin/mediamtx /home/deployuser/projects/video-stream-manager/mediamtx/mediamtx.yml
Restart=always
RestartSec=5
User=deployuser

[Install]
WantedBy=multi-user.target
```

## 技术栈

- **后端**: Go 1.23 + SQLite (modernc.org/sqlite + github.com/google/uuid)
- **流媒体**: MediaMTX (HLS/RTMP/WebRTC/SRT)
- **转码**: FFmpeg (libx264 + aac)
- **前端**: 纯 HTML + CSS + JS（无框架，无依赖）
- **数据库**: SQLite (WAL 模式)

## 版本历史

- **v0.3.0** — 音频支持、PTZ 云台控制、导播台、全屏/音量按钮
- **v0.2.0** — 抓拍功能、定时录制、推流管理
- **v0.1.0** — 基础架构、设备管理、实时监控
