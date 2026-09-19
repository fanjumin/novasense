# NovaSense — 感知网络

> **Nova**（新星）+ **Sense**（感知）= **感知新星**  
> 定位：**感知层（Perception Layer）**，而非监控

将旧手机、IP 摄像头、USB 摄像头、环境传感器统一接入，构建多设备感知网络。从底层设备感知 → 本地网关汇聚 → 云平台管理 → 远程查看，形成完整闭环。

---

## 仓库结构（monorepo）

> 2026-09-19 起，原 `fanjumin/novasense-gateway`、`fanjumin/novasense-agent` 与未纳管的插件/品牌目录合并为本单一仓库，完整提交历史已保留。

| 目录 | 组件 | 原仓库/来源 |
|------|------|------------|
| `gateway/` | 本地感知网关 | fanjumin/novasense-gateway（前身 video-stream-manager） |
| `agent/` | Android 感知节点 | fanjumin/novasense-agent（前身 netcam-android） |
| `server-plugin/` | EasyKai/VeroRun 云插件 | 原未纳管目录，本次入库 |
| `brand/` | 品牌视觉资产 | 原未纳管目录，本次入库 |
| `viewer/` | 远程查看器（预留） | 占位 |
| `docs/` | 技术白皮书与规划 | 本次入库 |

## 产品矩阵

| 产品 | 角色 | 技术栈 | 部署位置 |
|------|------|--------|----------|
| **[Gateway](#-novasense-gateway)** | 感知网关枢纽 | Go + SQLite + FFmpeg + MediaMTX | 本地主机（x86/RK3588/树莓派） |
| **[Agent](#-novasense-agent)** | Android 感知起点 | Kotlin + Camera1 + NanoHTTPD | Android 手机（旧手机复用） |
| **[Cloud](#-novasense-cloud)** | 云平台管理 | Python Flask / EasyKai Plugin | VPS / 公网服务器 |
| **[Viewer](#-novasense-viewer)** | 远程查看器 | *预留* | 多平台 |

---

## NovaSense Gateway

一站式感知网关枢纽。拉流 → 转码 → 分发 → 录制 → 抓拍 → 运动检测 → 人脸识别 → 告警通知 → 双向对讲，全部本地完成。

**目录：** `gateway/`

**核心能力：**

- 摄像头统一接入（RTSP / ONVIF / HTTP MJPEG / USB）
- FFmpeg 实时代理 + 自动重连（3s 恢复）
- HLS 实时播放（hls.js, 3-5s 延迟）+ WebRTC 低延迟（WHEP, <1s, 自动降级 HLS）
- PTZ 云台控制（海康 ISAPI/CGI，预置位 P1-P3）
- 导播台：多路画面同屏预览、PGM 切换、RTMP 推流到第三方平台
- 录制：手动 / 定时 / 运动触发，在线回放 + 下载
- 抓拍：定时抓拍 + 历史浏览
- 运动检测：FFmpeg scene 检测，灵敏度可调，SSE 实时推送 + 浏览器通知
- 人脸识别：纯 Go（pigo），感知哈希匹配，已知/陌生人自动区分
- 双向对讲：WebSocket → PCM → AAC → RTMP 推流，按住说话
- 设备发现：ARP 扫描 / ONVIF 探测 / SSDP / HTTP 指纹识别
- 设备分组管理
- 存储管理：保留天数 / 磁盘上限 / 自动清理
- Docker Compose 一键部署

**技术栈：** Go 1.25+ / SQLite / FFmpeg 7.0+ / MediaMTX / hls.js / pigo

```
┌──────────────────────────────────────────────────────────────────┐
│                    Go 后端 ( :8899 )                              │
│  设备管理 | FFmpeg管理 | 录制/定时 | 导播 | 人脸识别              │
│  SQLite DB | FFmpeg (多进程) | WebSocket SSE                      │
├──────────────────────────────────────────────────────────────────┤
│           MediaMTX ( :8888 / :1935 / :8554 / :8889 )              │
│  RTSP ← 拉流 | RTMP ← 推流 | HLS / WebRTC                        │
├──────────────────────────────────────────────────────────────────┤
│          Web 前端 ( SPA, 9 个 Tab )                               │
│  监控 | 录像 | 定时 | 抓拍 | 导播 | 运动 | 人脸 | 发现 | 状态     │
└──────────────────────────────────────────────────────────────────┘
```

---

## NovaSense Agent

将 Android 手机变成感知节点——IP 摄像头 + 环境传感器服务器。无云、无订阅，一部旧手机 + 浏览器即可工作。

**目录：** `agent/`

**核心能力：**

- MJPEG 视频流（浏览器直接看） + RTSP 流（VLC/FFmpeg）
- AAC 实时音频采集（HTTP + RTSP 双路）
- 前置/后置摄像头切换，1x-8x 数字变焦，闪光灯，镜像翻转
- 多分辨率支持（320x240 ~ 3840x2160），JPEG 质量/帧率可调
- 传感器数据叠加（OSD）：环境光、温度、气压、湿度、加速度计、陀螺仪、电池
- 网格运动检测（32×24 网格，灵敏度可调）
- Web 管理界面 + 完整 REST API
- 全部 HTTP API 控制（切换摄像头、变焦、曝光、白平衡等）

**技术栈：** Kotlin / Android Camera1 API / Jetpack Compose + Material 3 / NanoHTTPD / MediaCodec

```
         手机 → 摄像头画面 + 传感器数据
              │
              ▼
    HTTP MJPEG (:8080) ──→ 浏览器直接查看
    RTSP H.264 (:8554) ──→ Gateway / VLC 拉流
    REST API (:8080)    ──→ Gateway / Cloud 远程控制
```

---

## NovaSense Cloud

EasyKai（易开）平台感知网络插件。提供设备管理、RTMP 推流转发、HLS 实时观看、AI 画面分析、手机远程控制、订阅计费、PWA 远程 APP。

**目录：** `server-plugin/`（EasyKai 平台 `plugins/novasense/`）

**核心能力：**

- CRUD 设备管理 + 分组
- RTSP → RTMP 推流转发（ffmpeg 转码，自定义分辨率/码率）
- HLS 实时直播（hls.js 播放）
- AI 画面分析：移动检测截图 → LLM 分析 → 高置信度告警推送
- 手机远程控制：切换前后摄、闪光灯、镜像、分辨率、帧率、画质、变焦、曝光、白平衡
- 设备能力知识库：矩阵上报 + 跨用户聚合 + 平台同步
- 订阅计费 + API Key 鉴权
- PWA 远程 APP（Service Worker + manifest，添加到桌面即用）

**技术栈：** Python Flask / EasyKai Plugin Framework / ffmpeg / hls.js

```
手机 App → 能力矩阵 → VSM 网关 → POST /api/client/capability → 聚合 + 知识库
摄像头 → RTSP → ffmpeg → RTMP → VPS MediaMTX → HLS → 浏览器/PWA
移动检测 → 截图 → POST /api/client/analyze → LLMService → 高置信度告警
```

---

## NovaSense Viewer

*预留组件* — 多平台远程查看器，用于在手机、平板、桌面等设备上统一查看所有感知节点的实时画面和历史数据。

**目录：** `viewer/`（空目录，等待开发）

---

## 数据流全景

```
                       ┌──────────────┐
     ┌─────────────────│   用户浏览器   │◄────────────────┐
     │                 └──────┬───────┘                  │
     │                        │                          │
     ▼                        ▼                          │
┌──────────┐          ┌──────────────┐          ┌──────────────┐
│   Agent  │──RTSP──► │   Gateway    │──HLS──►  │   Cloud      │
│(Android) │──HTTP──► │  (本地枢纽)   │──SSE──►  │  (云端平台)   │
│  手机感知  │◄─API──│   :8899      │◄─API──│  easykai.cn  │
└──────────┘          └──────┬───────┘          └──────┬───────┘
                             │                         │
                             ▼                         ▼
                       ┌──────────────┐         ┌──────────────┐
                       │   MediaMTX   │         │  PWA Viewer  │
                       │ 流媒体服务器   │         │  远程APP     │
                       └──────────────┘         └──────────────┘
```

---

## 品牌视觉系统

| Token | 值 | 用途 |
|-------|------|------|
| `--ns-bg-deep` | `#0A0E14` | 深空黑背景 |
| `--ns-cyan` | `#00C8FF` | 星芒青蓝（主色） |
| `--ns-purple` | `#7C4DFF` | 星云紫（辅色） |
| `--ns-green` | `#69F0AE` | 在线/活跃 |
| `--ns-font-display` | Inter | 展示字体 |

完整 Design Tokens 见 `brand/tokens.css`

---

## 场景

- **无人零售**：3D 深度摄像头 + AI 分析，自动识别商品拿取
- **仓库安防**：旧手机 + IP 摄像头，低成本区域覆盖
- **远程看店**：旧手机作为感知起点，云端远程查看
- **家庭监护**：老人/儿童/宠物，多设备感知 + 告警
- **办公室管理**：工位占用检测、环境传感器联动
- **工地监控**：多路 RTSP 摄像头统一管理 + 录制
- **学校安防**：走廊 + 教室 + 门口多设备覆盖
- **教学演示**：手机摄像头当实物展台 + Gateway 导播推流

---

## 快速上手

```bash
# 1. 启动 Gateway（核心枢纽）
cd gateway
docker compose up -d
# 浏览器打开 http://localhost:8899 （首启为强制改密模式：出厂口令 admin 登录后即要求设置新管理口令）

# 2. 安装 Agent（手机感知节点）
# 在 Android 手机上安装 agent/app/build/outputs/apk/ 中的 APK
# 或编译：cd agent && ./gradlew assembleDebug

# 3. 连接 Cloud（云平台，可选）
# 将 server-plugin/ 部署到 EasyKai 平台的 plugins/novasense/ 目录下
```

---

## 许可证

各组件均为 MIT License。

Gateway：基于 [video-stream-manager](https://github.com/your-org/video-stream-manager)  
Agent：基于 [netcam-android](https://github.com/fanjumin/netcam-android)  
Cloud：NovaSense 云插件
