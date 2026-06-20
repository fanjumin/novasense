# 基于FFmpeg的视频流管理平台 — 技术方案

## 1. 项目概述

构建一个基于FFmpeg的视频管理软件，核心能力：

- **接入**：支持RTSP/ONVIF协议的监控摄像头、NVR设备
- **推流**：同时向抖音、快手、B站、YouTube等平台实时推流
- **录像**：本地/远程存储录制文件
- **检索**：按时间、设备、事件检索录像
- **管理**：设备管理、用户登录、权限控制

## 2. 系统架构

```
┌──────────────────────────────────────────────────────────┐
│                    Web UI (Vue3)                         │
│   设备管理 | 推流控制 | 录像检索 | 用户管理 | 系统监控    │
└──────────────┬───────────────────────────────────────────┘
               │ REST API + WebSocket
┌──────────────▼───────────────────────────────────────────┐
│              后端 API (Go/FastAPI)                        │
│   - 认证鉴权 (JWT)                                       │
│   - 设备CRUD                                            │
│   - 推流任务管理                                         │
│   - 录像任务管理                                         │
│   - 文件索引 & 检索                                      │
│   - 系统状态监控                                         │
└──────┬──────────────┬────────────────┬───────────────────┘
       │              │                │
┌──────▼──────┐ ┌─────▼──────┐ ┌──────▼────────────────┐
│  FFmpeg进程 │ │ MediaMTX   │ │   存储层               │
│  管理引擎   │ │(中转服务)  │ │   ┌──────────────┐    │
│  - 拉流     │ │ - RTSP     │ │   │ 本地磁盘/NFS │    │
│  - 转码     │ │ - RTMP     │ │   ├──────────────┤    │
│  - 推流     │ │ - HLS      │ │   │ 可选：S3/Minio│   │
│  - 录像     │ │ - SRT      │ │   └──────────────┘    │
└─────────────┘ └────────────┘ └────────────────────────┘
```

## 3. 核心模块设计

### 3.1 设备接入模块

**支持的设备类型：**

| 类型 | 协议 | 示例 |
|------|------|------|
| IP Camera | RTSP | 海康/大华/宇视等标准RTSP设备 |
| IP Camera | ONVIF | 通过ONVIF Profile S发现&配置 |
| NVR | RTSP/ONVIF | 通过NVR拉取多通道流 |
| USB摄像头 | V4L2 | 直接本地设备 |
| RTSP流 | RTSP | 任意标准RTSP源 |

**设备配置参数：**
```
- 名称 / 分组
- RTSP地址 (rtsp://user:pass@ip:554/stream1)
- ONVIF地址 (http://ip:80/onvif/device_service)
- 品牌/型号（自动探测）
- 分辨率/帧率偏好
- 状态（在线/离线/异常）
```

### 3.2 推流转发模块

**推流流程：**
```
摄像头RTSP源
    │ ffmpeg -rtsp_transport tcp -i <cam_rtsp>
    ▼
转码 (可选: 降分辨率/改码率/H.264/H.265)
    │
    ├──→ 中转流 → MediaMTX (本地RTMP/HLS调试)
    │
    ├──→ 抖音 RTMP: rtmp://live.douyin.com/live/<key>
    ├──→ 快手 RTMP: rtmp://rtmp.kuaishou.com/live/<key>
    ├──→ B站 RTMP: rtmp://live-push.bilivideo.com/live/<key>
    └──→ YouTube RTMP: rtmp://a.rtmp.youtube.com/live2/<key>
```

**推流任务配置：**
```json
{
  "id": "push_001",
  "device_id": "cam_001",
  "name": "正门直播推流",
  "platforms": ["douyin", "kuaishou", "bilibili", "youtube"],
  "platform_configs": {
    "douyin": { "rtmp_url": "rtmp://...", "stream_key": "..." },
    "kuaishou": { "rtmp_url": "rtmp://...", "stream_key": "..." },
    "bilibili": { "rtmp_url": "rtmp://...", "stream_key": "..." },
    "youtube": { "rtmp_url": "rtmp://...", "stream_key": "..." }
  },
  "transcode": {
    "video_codec": "h264",
    "width": 1920,
    "height": 1080,
    "fps": 30,
    "bitrate": "4000k",
    "preset": "veryfast"
  },
  "enabled": true,
  "status": "running"
}
```

**FFmpeg单设备→多平台推流命令示例：**
```bash
# 单进程多路推流（复用解码流，节省CPU）
ffmpeg -rtsp_transport tcp -i "rtsp://user:pass@192.168.1.100:554/stream1" \
  -c:v libx264 -preset veryfast -b:v 4000k -maxrate 4000k -bufsize 8000k \
  -c:a aac -b:a 128k -ar 44100 -ac 2 \
  -f tee -map 0:v -map 0:a \
  "[f=flv]rtmp://live.douyin.com/live/key1|[f=flv]rtmp://rtmp.kuaishou.com/live/key2|[f=flv]rtmp://live-push.bilivideo.com/live/key3|[f=flv]rtmp://a.rtmp.youtube.com/live2/key4"
```

或为每个平台独立进程（便于独立启停/重连）：
```bash
# 每个平台一个ffmpeg进程，用本地MediaMTX做一次中转
# 步骤1：拉RTSP推到本地MediaMTX
ffmpeg -rtsp_transport tcp -i "rtsp://..." -c copy -f rtsp rtsp://localhost:8554/cam1

# 步骤2：从本地MediaMTX拉流分推各平台
ffmpeg -i rtsp://localhost:8554/cam1 -c copy -f flv rtmp://live.douyin.com/live/key1
ffmpeg -i rtsp://localhost:8554/cam1 -c copy -f flv rtmp://a.rtmp.youtube.com/live2/key2
```

### 3.3 录像存储模块

**录像模式：**

| 模式 | 说明 | 存储格式 |
|------|------|----------|
| 连续录像 | 24/7不间断录制 | 分段MP4 (5/10/30min) |
| 事件录像 | 移动检测/报警触发录制 | 事件分段MP4 |
| 定时录像 | 按时间表录制 | 分段MP4 |
| 回放缓冲 | 预录N秒+事件后N秒 | 内存→磁盘 |

**存储策略：**
```
存储路径: /data/videos/{device_id}/{YYYYMMDD}/{HHMMSS}_{event_id}.mp4
索引数据库: SQLite/PostgreSQL
  - file_path, device_id, start_time, end_time, size, video_codec, has_motion, event_type

自动清理: 按天数/容量保留，可配置
```

**FFmpeg录像命令：**
```bash
# 连续分段录制
ffmpeg -rtsp_transport tcp -i "rtsp://..." \
  -c copy -map 0 \
  -f segment -segment_time 600 -reset_timestamps 1 \
  -strftime 1 "/data/videos/cam1/%Y%m%d/%H%M%S.mp4"

# 事件录制（带时间戳水印 + 移动检测过滤）
ffmpeg -rtsp_transport tcp -i "rtsp://..." \
  -filter:v "drawtext=text='%{localtime}':x=10:y=10:fontsize=24:fontcolor=white" \
  -c:v libx264 -preset fast -crf 23 \
  -c:a aac -b:a 128k \
  /data/videos/cam1/event_$(date +%Y%m%d_%H%M%S).mp4
```

### 3.4 视频检索模块

**检索维度：**
- 时间范围（起止时间）
- 设备ID / 设备分组
- 事件类型（移动检测/报警/定时）
- 录像时长
- 标签/备注

**检索结果：**
```json
{
  "results": [
    {
      "id": "rec_001",
      "device_name": "正门摄像头",
      "start_time": "2026-06-18T10:30:00",
      "end_time": "2026-06-18T10:35:00",
      "duration": 300,
      "file_path": "/data/videos/cam1/20260618/103000.mp4",
      "size": 150000000,
      "has_motion": true,
      "event_type": "motion",
      "tags": ["人员进出"]
    }
  ]
}
```

**播放方式：**
- 直接下载MP4文件播放
- HLS实时转码流播放（浏览器可直接看）
- 截图预览（FFmpeg抽取关键帧生成缩略图）

### 3.5 用户与权限模块

**用户角色：**

| 角色 | 权限 |
|------|------|
| 管理员 | 全部权限 |
| 运营 | 推流控制、设备查看 |
| 查看者 | 仅查看直播/录像 |
| API用户 | 通过API Key访问 |

**认证方式：**
- JWT Token（Web登录）
- API Key（第三方集成）

## 4. 技术栈

| 层级 | 选型 | 理由 |
|------|------|------|
| 后端框架 | Go (Gin/Echo) 或 Python FastAPI | Go性能好、内存低、适合进程管理；FastAPI开发快 |
| 流媒体中转 | MediaMTX | 成熟、高性能、支持RTSP/RTMP/HLS/SRT/WebRTC |
| 数据库 | PostgreSQL + Redis | PG存储设备/用户/录像索引，Redis做任务队列 |
| 前端 | Vue3 + Element Plus + HLS.js | 成熟、组件丰富 |
| FFmpeg管理 | Go 的 os/exec 或 Python subprocess | 直接管理进程生命周期 |
| 任务调度 | 内置定时器 + goroutine/asyncio | 定时录像、巡检、清理 |
| 部署 | Docker Compose | 一键部署 |
| 存储 | 本地磁盘 + 可选S3/MinIO | 灵活 |

## 5. 数据结构设计

### devices (设备表)
```sql
CREATE TABLE devices (
  id          UUID PRIMARY KEY,
  name        VARCHAR(128) NOT NULL,
  group_name  VARCHAR(64),
  protocol    VARCHAR(16) NOT NULL,  -- rtsp / onvif / v4l2
  url         TEXT NOT NULL,         -- RTSP地址
  username    VARCHAR(64),
  password    VARCHAR(256),          -- AES加密存储
  brand       VARCHAR(64),
  model       VARCHAR(64),
  status      VARCHAR(16) DEFAULT 'offline',  -- online/offline/error
  enabled     BOOLEAN DEFAULT true,
  metadata    JSONB,
  created_at  TIMESTAMP DEFAULT NOW(),
  updated_at  TIMESTAMP DEFAULT NOW()
);
```

### push_tasks (推流任务表)
```sql
CREATE TABLE push_tasks (
  id              UUID PRIMARY KEY,
  device_id       UUID REFERENCES devices(id),
  name            VARCHAR(128),
  platform        VARCHAR(32),     -- douyin/kuaishou/bilibili/youtube
  rtmp_url        TEXT NOT NULL,
  stream_key      TEXT NOT NULL,
  transcode_config JSONB,           -- 分辨率/码率/编码
  status          VARCHAR(16) DEFAULT 'stopped',  -- stopped/running/error
  pid             INTEGER,          -- ffmpeg进程ID
  last_error      TEXT,
  created_at      TIMESTAMP DEFAULT NOW(),
  updated_at      TIMESTAMP DEFAULT NOW()
);
```

### recordings (录像记录表)
```sql
CREATE TABLE recordings (
  id          UUID PRIMARY KEY,
  device_id   UUID REFERENCES devices(id),
  file_path   TEXT NOT NULL,
  start_time  TIMESTAMP NOT NULL,
  end_time    TIMESTAMP,
  file_size   BIGINT,
  duration    INTEGER,              -- 秒
  video_codec VARCHAR(16),
  width       INTEGER,
  height      INTEGER,
  fps         REAL,
  has_motion  BOOLEAN DEFAULT false,
  event_type  VARCHAR(32),          -- continuous / motion / schedule / manual
  tags        TEXT[],               -- 标签
  created_at  TIMESTAMP DEFAULT NOW()
);
```

### users (用户表)
```sql
CREATE TABLE users (
  id          UUID PRIMARY KEY,
  username    VARCHAR(64) UNIQUE NOT NULL,
  password    VARCHAR(256) NOT NULL,  -- bcrypt hash
  role        VARCHAR(16) DEFAULT 'viewer',  -- admin/operator/viewer
  nickname    VARCHAR(64),
  email       VARCHAR(128),
  enabled     BOOLEAN DEFAULT true,
  created_at  TIMESTAMP DEFAULT NOW()
);
```

## 6. 部署方案

### Docker Compose
```yaml
services:
  mediamtx:
    image: bluenviron/mediamtx
    ports:
      - "8554:8554"   # RTSP
      - "1935:1935"   # RTMP
      - "8888:8888"   # HLS
    volumes:
      - ./mediamtx.yml:/mediamtx.yml

  api:
    build: ./backend
    ports:
      - "8080:8080"
    volumes:
      - /data/videos:/data/videos
      - /var/run/docker.sock:/var/run/docker.sock  # 管理FFmpeg容器
    depends_on:
      - postgres
      - mediamtx

  frontend:
    build: ./frontend
    ports:
      - "3000:3000"

  postgres:
    image: postgres:16
    volumes:
      - pgdata:/var/lib/postgresql/data

  minio:
    image: minio/minio
    ports:
      - "9000:9000"   # API
      - "9001:9001"   # Console
```

## 7. 安全考虑

- 设备密码：AES-CBC加密存储，运行期解密
- API认证：JWT + API Key 双通道
- RTMP推流：支持SRT加密传输
- 录像存储：可选加密（GPG/AES）
- HTTPS：反向代理（Nginx/Caddy）暴露服务
- 用户密码：bcrypt 哈希
- 操作日志：所有推流/配置变更留痕

## 8. 开发计划

| 阶段 | 内容 | 预估工时 |
|------|------|----------|
| P0 | DB设计 + 用户注册登录 + 设备CRUD + FFmpeg拉流转RTMP | 1周 |
| P1 | 多平台推流管理 + MediaMTX集成 + 实时状态WebSocket | 1周 |
| P2 | 录像模块（分段录制 + 自动清理 + 存储策略） | 1周 |
| P3 | 录像检索 + 缩略图 + HLS在线回放 | 1周 |
| P4 | 系统监控面板（设备状态/推流健康/存储用量） | 3天 |
| P5 | Docker部署 + 文档 + 测试 | 2天 |

**总计：约4-5周（单人全职开发）**

## 9. 边界与注意事项

- 抖音/快手/B站等平台推流地址和key有有效期，需定期更新
- 上行带宽要求：1路1080p@30fps推流约4-6Mbps，N路推流 = N × 单路带宽
- 存储估算：1路1080p@4Mbps，24h ≈ 43GB，30天 ≈ 1.3TB
- CPU负载：软件编码（libx264）单路1080p推流约占用1-2核CPU，建议使用GPU加速（NVENC/VAAPI）
- 部分平台（如抖音）有推流认证鉴权机制，需要适配

## 10. 推荐实施路线

**最小可行版本（MVP）**：

1. 后端 Go 或 Python API（设备管理 + 推流控制 + 录像）
2. FFmpeg进程管理器（拉起/监控/重启推流进程）
3. MediaMTX做本地流中转
4. Web UI（设备列表 + 推流On/Off + 录像回看）
5. SQLite起步（后期可升级PG）

**第一天可交付**：命令行版——API + FFmpeg进程管理，通过curl测试推流。第二天加Web界面。
