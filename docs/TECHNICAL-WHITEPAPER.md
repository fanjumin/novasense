# NovaSense 技术白皮书

> **NovaSense · 感知网络（Constellation of Perception）**
> 版本口径：**v0.8.4（全线统一，2026-09-19 完成收敛）**｜ 撰写日期：2026-09-19
> 定位声明：**感知层（Perception Layer）**，而非监控系统

本白皮书对 NovaSense 产品线代码库（`F:\projects\novasense`）的五个组成目录进行逐文件技术剖析，覆盖系统定位、总体架构、各组件内部实现、数据流与流媒体管线、工程亮点，以及对"文档声称能力"与"代码实际实现"之间的差异做严谨核对。所有关键结论均标注来源文件路径（`目录:行号` 或 `文件`），以便第三方审阅复核。

---

## 目录

1. [产品概述与设计哲学](#1-产品概述与设计哲学)
2. [仓库结构与组件全景](#2-仓库结构与组件全景)
3. [总体系统架构](#3-总体系统架构)
4. [NovaSense Gateway — 本地感知网关](#4-novasense-gateway--本地感知网关)
5. [NovaSense Agent — Android 感知节点](#5-novasense-agent--android-感知节点)
6. [NovaSense Server Plugin — 云平台插件](#6-novasense-server-plugin--云平台插件)
7. [NovaSense Brand — 品牌视觉系统](#7-novasense-brand--品牌视觉系统)
8. [NovaSense Viewer — 远程查看器（预留）](#8-novasense-viewer--远程查看器预留)
9. [跨组件事件与告警机制](#9-跨组件事件与告警机制)
10. [安全模型与鉴权](#10-安全模型与鉴权)
11. [部署形态](#11-部署形态)
12. [现状评估：README 声称 vs 代码实际](#12-现状评估readme-声称-vs-代码实际)
13. [技术债务清单](#13-技术债务清单)
14. [结论](#14-结论)
15. [附录：关键源码索引](#附录关键源码索引)

---

## 1. 产品概述与设计哲学

### 1.1 一句话定义

NovaSense 将**旧手机、IP 摄像头、USB 摄像头、环境传感器**统一接入，构建多设备感知网络；数据从底层设备感知 → 本地网关汇聚 → 云平台管理 → 远程查看，形成完整闭环（`README.md:6`）。

### 1.2 品牌与定位哲学

NovaSense 的产品自我定位刻意与"监控（surveillance）"划清界限，而是强调"感知层（Perception Layer）"这一基础设施隐喻（`README.md:3-4`）。品牌叙事把每一台接入设备比作暗夜中的星，被网络点亮后爆发为新星（Nova），众多节点连成"感知星座"（`NOVA-VISUAL-PLAN.md:5-9`）。这一哲学直接塑造了其技术特征：

- **边缘优先（Edge-first）**：拉流、转码、分发、检测、识别、告警等全部能力设计为在本地 Gateway 完成，云只做管理与展示（`README.md:23`）。
- **异构统一**：以 FFmpeg + MediaMTX 为"万能适配器"，把 RTSP / HTTP-MJPEG / USB(V4L2) / 私有协议 / 手机 JPEG 流归一为统一的 HLS / WebRTC / RTSP 分发。
- **零订阅、零云依赖**：单台旧手机 + 一个浏览器即可本地工作（`README.md:64`）。

### 1.3 产品矩阵

| 组件 | 角色 | 技术栈 | 部署位置 | 目录 |
|------|------|--------|----------|------|
| **Gateway** | 感知网关枢纽（本地） | Go + SQLite + FFmpeg + MediaMTX | x86/RK3588/树莓派/任意 ARM64 | `novasense-gateway/` |
| **Agent** | Android 感知起点 | Kotlin + Camera1 + NanoHTTPD | 旧手机/旧平板 | `novasense-agent/` |
| **Server Plugin** | 云平台管理 | Python Flask / EasyKai Plugin | VPS / 公网服务器 | `novasense-server-plugin/` |
| **Viewer** | 远程查看器 | *规划中，未开发* | 多平台 | `novasense-viewer/`（空） |
| **Brand** | 品牌视觉系统 | CSS Tokens + SVG + HTML | 设计资产 | `novasense-brand/` |

> 说明：README 将第三组件称为 "NovaSense Cloud"（`README.md:16`），实际代码目录为 `novasense-server-plugin/`，是一个挂载在 EasyKai/VeroRun 平台下的 Flask 插件，能力集小于 README 的宣称（详见 §12）。

---

## 2. 仓库结构与组件全景

### 2.1 目录树（逻辑）

> **2026-09-19 更新**：下列目录树描述的是**合并前**的本地聚合工作区。此后三仓与未纳管目录已合并为单一仓库 `fanjumin/novasense`（完整历史保留），映射为 `novasense-gateway/→gateway/`、`novasense-agent/→agent/`、`novasense-server-plugin/→server-plugin/`、`novasense-brand/→brand/`、`novasense-viewer/→viewer/`，本文与规划文档移入 `docs/`。正文中形如 `backend/main.go`、`frontend/index.html` 的引用均相对 `gateway/`。

```
F:\projects\novasense\                ← 聚合工作区（非 git 仓库）
├── README.md                         ← 产品线总览
├── NOVASENSE_PLAN.md                  ← 三仓统一品牌规划（NetCam/VSM → NovaSense）
├── NOVA-VISUAL-PLAN.md                ← 视觉系统规划
├── novasense-gateway\        [git]    ← Go 本地网关（核心枢纽，代码量最大）
│   ├── backend\        *.go + cmd/ + sdk/    7044 行 Go（不含 cmd/sdk）
│   ├── frontend\       index.html SPA(9 Tab) + phone/ + panorama + js/
│   ├── ruview-ui\      WiFi-CSI 感知前端 fork（ES Module + RN/Expo）
│   ├── mediamtx\       MediaMTX 配置
│   ├── deploy\         systemd unit + setup.sh
│   ├── scripts\        迁移/辅助脚本
│   └── docker-compose.yml + Dockerfile
├── novasense-agent\         [git]     ← Android 采集 App
│   └── app\src\main\  11 个 Kotlin 文件（2977 行）+ modern 冗余（1401 行）
├── novasense-server-plugin\ [未纳管]  ← EasyKai Flask 插件（6 文件 / 约 941 行）
├── novasense-brand\         [未纳管]  ← 品牌资产（12 文件）
└── novasense-viewer\        [空]      ← 预留组件
```

### 2.2 Git 纳管现状

- **根目录 `F:\projects\novasense` 不是 git 仓库**——它是把三个独立历史仓库聚合在一起的工作区。
- `novasense-agent`、`novasense-gateway` 各自含 `.git`，是**独立仓库**。
- `novasense-server-plugin`、`novasense-brand`、`novasense-viewer` **均未纳入版本控制**。

品牌重构轨迹（跨仓库一致）：`NetCam / video-stream-manager → NovaSense` 的统一改名（gateway 提交 `6b69aaa rebrand`、agent 提交 `cde4789 重命名: NetCam Pro → NovaSense Agent`），三仓版本口径统一重置为 v0.1.0（`NOVASENSE_PLAN.md:14-16`、`50-61`）。

---

## 3. 总体系统架构

### 3.1 分层架构（brand 架构图的工程表达）

`novasense-brand/architecture.html`（215 行，纯 CSS 三层图）以视觉方式定义了系统的感知能力分层（§2.4 转述）：

```
┌─────────────────────────────────────────────────────────────┐
│  感知层 SENSING LAYER                                          │
│  旧手机/平板(摄像头+温湿度/气压/加速度/陀螺/麦克风) · IP摄像头  │
│  (RTSP/ONVIF) · USB摄像头(V4L2) · 3D深度摄像头(RealSense/      │
│  Kinect·无人购物) · 环境传感器(BLE/Zigbee)                     │
└───────────────────────────▼─────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│  NovaSense Gateway  本地感知网关                                │
│  汇聚所有传感器数据 · 运行 AI 推理 · 控制推流与存储             │
│  硬件：x86 Linux / RK3588 / 树莓派 / Jetson / 任意 ARM64       │
├─────────────────────────────────────────────────────────────┤
│  EDGE AI 感知能力                                              │
│  人脸识别 · 人体检测 · 3D空间感知 · 环境分析 · 无人购物 ·       │
│  边缘推理(NPU/CUDA·不依赖云) · 多传感器融合                     │
└───────────────────────────▼─────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│  云平台 CLOUD                                                  │
│  NovaSense Cloud(设备管理/多租户/API Key/License) ·            │
│  NovaSense Viewer(移动端/推送/画中画) · AI服务(云端扩展/训练)   │
└─────────────────────────────────────────────────────────────┘
```

> 注意：该架构图描绘的是**产品愿景**（含 3D 深度感知、无人购物、边缘 NPU 推理等），其中多项尚未在当前代码中落地（详见 §12）。图中亦未画出 EasyKai 插件层，插件形态在架构图中缺席。

### 3.2 运行时数据流全景

实际可运行的数据流（据代码核实）如下——这是本白皮书的"事实基线"：

```
                                    用户浏览器
                                        │
        ┌───────────────────────────────┼───────────────────────────────┐
        ▼                               ▼                               ▼
  ┌───────────┐  RTSP/HTTP   ┌────────────────┐  HLS/WHEP/SSE  ┌──────────────┐
  │  Agent    │────────────► │    Gateway     │──────────────► │ Server Plugin│
  │ (Android) │◄────API───── │   (Go :8899)   │◄────API+Cookie─│  (EasyKai)   │
  │ :8080/8554│              │  FFmpeg 多进程  │                │ /plugin/     │
  └───────────┘              │  SQLite + SSE  │                │  novasense   │
        ▲                    ├────────────────┤                └──────┬───────┘
        │                    │  MediaMTX      │                       │
        └─── 手机帧/JPEG ─── │  RTSP:8554     │                       ▼
        远程控制            │  RTMP:1935     │                 浏览器 / PWA
                            │  HLS:8888      │
                            │  WebRTC:8889   │
                            └────────────────┘
                                     │
                                     ▼ (可选并置)
                            ┌──────────────────────┐
                            │ RuView WiFi-CSI 感知  │
                            │ :3000 http / :3001 ws │
                            └──────────────────────┘
```

统一模式：**任何源 → 本地 FFmpeg 转码 → RTMP 推入 MediaMTX → 多协议分发**（`backend/proxy.go:168-236`）。

---

## 4. NovaSense Gateway — 本地感知网关

Gateway 是整个产品线的核心枢纽，也是代码量最大、最成熟的组件。它是**单进程 Go 服务**（监听 `:8899`，`backend/main.go:2366`），配 SQLite（WAL 模式，`backend/store.go:97`）、FFmpeg 多进程与 MediaMTX 流媒体服务器，可选并置 RuView WiFi 感知容器。

### 4.1 内部分层

| 层 | 职责 | 关键实现 |
|----|------|----------|
| HTTP 层 | 路由分发、CORS、Cookie 会话鉴权 | `Server` 结构体聚合 store/ffmpeg/director/discovery/faceDetector/SSE 广播（`main.go:37-51`）；`ServeHTTP`（`main.go:53-79`） |
| 业务层 | 设备 CRUD、代理启停、录制、抓拍、定时计划、PTZ、导播、人脸、对讲、发现、许可证、存储管理 | 见 §4.2 源文件职责 |
| 进程层 | FFmpeg 子进程生命周期管理 | `FFmpegManager`；并发闸门 `maxConcurrentFFmpeg=4`（`main.go:30`）；退避重连 3s→60s（`proxy.go:401-424`）；15s 健康检查、连续 5 次无流强杀自愈（`main.go:949-1012`） |
| 数据层 | 裸 SQL CRUD + 迁移、文件静态服务 | `store.go`；静态服务 `/videos/`、`/snapshots/`、`/faces/`（`main.go:2186-2200`） |
| 后台循环 | 异步任务 | 运动检测器(10s)、调度器(30s)、抓拍器(10s)、录像清理(6h)、订阅心跳(24h)（均在 `NewServer` 启动，`main.go:2202-2222`） |

### 4.2 源文件职责

Go 后端 `backend/`（不含 `cmd/`、`sdk/`）约 7044 行。

| 文件 | 行数 | 职责要点 |
|------|------|----------|
| `main.go` | 2373 | 路由注册、设备/录制/抓拍/状态/导入导出、SSE、对讲、PTZ、运动检测、手机帧接收、V4L2、许可证、心跳与 AI 分析上云 |
| `store.go` | 1063 | SQLite 全部 CRUD + 9 张表迁移 |
| `discovery.go` | 1253 | 8 阶段设备发现（ARP/SSDP/ONVIF/端口/HTTP指纹/RTSP猜径/类型推断/USB），内置 MAC OUI 厂商库与置信度评分 |
| `proxy.go` | 600 | FFmpegManager：分辨率探测与分档码率、代理参数构建、录制、WAPA FIFO 管道 |
| `camera_settings.go` | 589 | 图像参数三通道读写：雄迈 SDK → ONVIF GetImagingSettings → 厂商 CGI；含 HTTP Digest 认证 |
| `director.go` | 469 | 导播台：多源切换、overlay（drawtext/logo/pip）、RTMP 双输出 tee |
| `face_detector.go` | 321 | pigo 级联检测 + aHash 感知哈希匹配 |
| `ruview_bridge.go` | 127 | RuView 状态与 WebSocket 桥（ticket 铸票） |
| `auth.go` | 77 | 会话鉴权、白名单、默认密码告警 |
| `sdk_xm.go` + `xm_sdk_bridge.c` + `sdk/` | — | **雄迈(XiongMai) NetSDK**（CGO 链接 `libxmnetsdk.so`），读写亮度/对比度/饱和/锐度 |
| `cmd/wapa-pull/main.go` | 132 | WAPA"波粒"纯数字摄像头私有协议（TCP:9001）拉流，为裸 MPEG-4 VOP 补 VOL/VOS 头 |
| `facefinder` | 239KB | pigo 级联分类器二进制 |

> **代码组织状态**：`auth/proxy/camera_settings/hls/director` 等文件是近期从单体 `main.go.bak`（81KB、3000+ 行）拆出的；仓库存在 `.split_needs_work` 标记（内容仅 "Need proper file splitting"），且 FFmpegManager 的健康检查/指标仍留在 `main.go`（`main.go:949-1041`）——**拆分处于半途**。

### 4.3 流媒体管线（核心）

Gateway 的技术护城河在于把异构设备统一为可分发的标准流。管线如下：

**① 拉流（多源接入）**
- RTSP(TCP)、HTTP-MJPEG、USB(V4L2 mjpeg 15fps)、WAPA(FIFO→ffmpeg `-f m4v pipe`)、手机 JPEG POST 流（`main.go:1466-1486`）。

**② 转码**
- `libx264 veryfast + zerolatency`、GOP 8、25fps、按分辨率分档码率（1000k–12000k，`proxy.go:89-166`）、AAC 64k/16k 单声道；推流至 `rtmp://127.0.0.1:1935/live/{deviceId}`（`proxy.go:168-236`）。

**③ 分发（MediaMTX，host 网络）**

| 协议 | 端口 | 说明 |
|------|------|------|
| RTSP | 8554 | 拉流源 |
| RTMP | 1935 | FFmpeg 推入 MediaMTX |
| HLS | 8888 | `hlsAlwaysRemux`、mpegts、500ms 分段 → 约 1.5–3s 延迟 |
| WebRTC | 8889 | WHEP 低延迟（<1s），ICE UDP 8189 |
| SRT / MoQ | 8890 / 8892 | `auto.crt/key` 为 MoQ 自签 ECDSA 证书（CN=mediamtx） |

**④ 播放策略**：前端先起 HLS，再异步尝试 WHEP 升级，失败则回退 HLS（`frontend/index.html:873-883, 974-1010`）；Web 经 `/hls/` 反代避免跨域（`hls.go:49`）。

**⑤ 旁路**：录制/运动录像以 `-c copy` 直存 mp4；导播台独立 FFmpeg 进程输出 `live/director` 并可 tee 外推第三方 RTMP。

### 4.4 API 概览

除 `/api/login`、`/api/health`、`/api/events`、`/api/license/check`、`/hls/*`、`/ui/login.html` 需免鉴权白名单外，其余路由均需 session cookie（`auth.go:59-67`）。路由注册集中于 `main.go:133-185`。

主要端点分组：设备 `GET/POST /api/devices`、`GET/PUT/DELETE /api/devices/{id}`；代理 `POST /api/proxy/start/{id}`（支持 width/height/bitrate/vf 参数）；录制 `GET/POST/DELETE /api/recordings`；抓拍、定时计划、PTZ（`/api/ptz/{id}/{action}` + 预置位）、运动（config/events）、导播、人脸（`/api/faces`、`/api/face-events`）、存储、分组、V4L2、发现（5 连）、许可证（generate/check/bind/list）、RuView 桥（`/api/ruview/status`、`/api/ruview/ws`）、手机透传（`/api/phone/{id}/{cmd}`）、状态/健康/指标/导入导出。完整路由表见附录 A。

> **工程注意**：手动录制 `POST /api/recordings` 是**同步阻塞 HTTP**——请求会挂到录制时长结束才返回（`proxy.go:497`），README 未提示此行为。

### 4.5 智能能力

- **运动检测**：FFmpeg scene 检测（`main.go:1360-1366`），灵敏度可调，事件入库并经 SSE 实时推送。
- **人脸识别**：pigo 级联检测（最小脸 40px，IoU 聚类 0.2）→ 裁剪缩略图 → aHash 64 位感知哈希 → Hamming 距离 ≤15 匹配（`face_detector.go:321`、`main.go:2102`）。流程：运动抓拍→检脸→匹配/登记 unknown→写 face_events→SSE。
- **双向对讲**：浏览器 PCM(s16le/16k/mono) → WebSocket → FFmpeg → AAC → RTMP `talk/{id}`，摄像头回拉（`main.go:1896-1905`）。
- **PTZ**：海康 ISAPI + CGI 回退，预置位 P1–P3，500ms 自动停止（`main.go:1045-1171`）。**注意：PTZ 非 ONVIF 标准**，ONVIF 仅用于发现与图像参数探测。

### 4.6 设备发现

`discovery.go`（1253 行）实现 8 阶段扫描：ARP(`/proc/net/arp`) → SSDP(239.255.255.250:1900) → ONVIF WS-Discovery(3702) → 端口扫描(14 个摄像头端口) → HTTP 指纹(Server 头/title/特征串) → RTSP 路径猜测(ffprobe 验证) → 设备类型推断 → USB `/dev/video0-9`。内置海康/大华/萤石/雄迈等 MAC OUI 库与置信度评分（10–90）。

### 4.7 RuView WiFi 感知集成

`ruview-ui/` 是 WiFi-DensePose(RuView) 上游 UI 的定制 fork（ES Module 架构 + React Native/Expo 子项目），页面含 dashboard、observatory（呼吸/跌倒/入侵场景轮播）、pose-fusion（视频+CSI 双模态姿态融合）。它通过两条代理路由接入 Gateway：`/api/ruview/status` → `http://127.0.0.1:3000/api/v1/status`；`/api/ruview/ws` 先 POST 铸一次性 ticket（ADR-272）再桥接 `ws://127.0.0.1:3001/ws/sensing?ticket=`（`ruview_bridge.go:127`）。前端 `frontend/js/ruview3d.js` 用 Three.js 做高斯泼溅渲染。

---

## 5. NovaSense Agent — Android 感知节点

Agent 把 Android 旧手机改造成"摄像头 + 环境传感器"服务器，在局域网内直接对外提供 MJPEG（`:8080`）、RTSP H.264+AAC（`:8554`）与 REST API（`README.md:64-89`）。项目由 NetCam Pro 更名而来，全部代码在主源码 11 个 Kotlin 文件（2977 行）+ `modern` 冗余副本（1401 行），Kotlin 合计约 4378 行。

### 5.1 App 架构（三层）

- **MainActivity（1050 行）**：Compose 单 Activity，直接持有 Camera1 实例（`MainActivity.kt:78`），在 `setPreviewCallback` 帧回调内完成"取帧→JPEG→镜像→运动检测→OSD/水印→广播"全流水线（`MainActivity.kt:308-354`），并轮询 `CameraService.commandQueue` 应用远程控制（`:310-325`）。
- **CameraService（232 行）**：`LifecycleService` 前台服务（Android 10+ 声明 `FOREGROUND_SERVICE_TYPE_CAMERA | MICROPHONE`，`CameraService.kt:114-117`），`START_STICKY`，按"HTTP→RTSP→H264→音频"顺序**独立启动各组件、单个失败不阻塞其余**（`:127-175`），通过 `broadcastFrame(jpeg)` 把帧分发出去（`:185-190`）。
- **NovaSenseApp（35 行）**：Application，仅创建低优先级通知渠道。

**远程控制模型**为生产者-消费者解耦：HTTP 线程 `enqueueCommand(key,value)` 写 `ConcurrentLinkedQueue`，渲染帧回调线程排空并回主线程应用（`CameraService.kt:62-70`）——避免跨线程操作 Camera1。

### 5.2 视频管线

**MJPEG 路（已工作）**：Camera1 按曼哈顿距离选最接近的预览档位，格式固定 NV21，每帧 `YuvImage.compressToJpeg`（`MainActivity.kt:327-329`）→ 镜像/OSD/水印各再解码重编码 → broadcast。HttpServer 的 MJPEG 是自管理推流：`MjpegBody : InputStream` 内含 `LinkedBlockingQueue(3)`，**慢消费者队满即被踢出**（`HttpServer.kt:25-92`），响应为 `multipart/x-mixed-replace` chunked 流。

**H.264/RTSP 路（骨架完整但断链）**：`H264Encoder`（158 行）用 MediaCodec Surface 输入模式（I 帧间隔 1s、默认 2Mbps），输出侧解析 `csd-0/csd-1` 得 SPS/PPS、按 Annex-B 切分 NAL（`H264Encoder.kt:61-155`）。**但 `getInputSurface()` 全工程无调用者、`drainEncoder` 无调用者、无异步 callback**——即编码器 Surface 从未与相机预览对接，RTSP 视频通路实际不出流。

### 5.3 RTSP 服务器

`RtspServer`（507 行）手写 RTSP/1.0：`ServerSocket` 单线程 accept、**仅支持单客户端**（`RtspServer.kt:68-94`），支持 OPTIONS/DESCRIBE/SETUP/PLAY/TEARDOWN。SDP 声明 H264 PT96 + AAC PT97。SETUP **只接受 RTP/AVP/TCP interleaved**（`:230-258`），UDP 分支为占位。打包：H264 单 NAL 超 MTU 走 RFC3555 FU-A 分片，AAC 走 hbr AU-header。**已知限制**：时间戳为固定步进（视频每 NAL +3000 = 1/30s，与 15fps 编码目标不符，`:393`）、SSRC 硬编码 `0x12345678`、无 RTCP。

### 5.4 音频管线

`AudioCapture`（145 行）：`AudioRecord` MIC 44100Hz/单声道/PCM16 → MediaCodec AAC-LC 128kbps，每帧双路分发到 `latestAacFrame`（HTTP 路）+ `rtspServer.queueAacFrame`（RTSP 路）（`CameraService.kt:167-170`）。**但 HTTP `/audio.aac` 是未完成桩**：`read()` 恒返回 -1（`HttpServer.kt:219-241`）。

### 5.5 传感器、OSD 与运动检测

`SensorCollector`（173 行）注册 6 类传感器（光照/温度/气压/湿度/加速度计/陀螺仪）+ 电池（sticky 广播），以 `StateFlow<SensorData>` 暴露（`SensorCollector.kt:56-57`）。OSD 在 `MainActivity.overlaySensorData`（`:430-483`）绘制时间戳/电量/环境读数/分辨率帧率。

**运动检测**（`MainActivity.kt:382-428`）：JPEG 解码 → `ALPHA_8` 灰度 → 降采样为 **32×24 网格**（768 格均值）→ 与上一帧逐格差值，超阈值（默认 30）记为变化格 → 变化格数 > 5%×768≈38 时置位，经 `/status` JSON 暴露（仅后置摄像头，纯瞬时布尔标志，无事件历史）。

### 5.6 HTTP API

HttpServer（417 行）路由集中于 `HttpServer.kt:94-126, 317-387`：`/`（内嵌暗色 Web 控制台）、`/video`+`/mjpeg`、`/shot.jpg`、`/audio.aac`（桩）、`/status`、`/debug`、`/api/restart`、`/api/settings`（stub）、`/api/cam_switch`、`/api/torch`、`/api/mirror`、`/api/zoom`(1-8x)、`/api/quality`、`/api/fps`、`/api/resolution`、`/api/exposure`+`/api/white_balance`（入队但 `applyCameraSettings` 无对应分支，**静默丢弃**）。

### 5.7 授权模型（空壳）

`License.kt`（25 行）：`IS_PRO` 及 `ENABLE_TASKER/ONVIF/CLOUD` 全部是**编译期常量 false**；`checkLicense` 为**离线占位实现**——非空即返回 true 并提示"验证成功 (离线模式)"，不做任何网络校验，也不改变 `IS_PRO`（`License.kt:17-24`）。UI 仍有"平台地址+授权码"表单，`platformUrl`/`licenseKey` 持久化于 DataStore（`SettingsStore.kt:109-112`）——云端激活仅为预留接口。

> **重要副作用**：传感器 OSD 仅在 `IS_PRO=true` 时启用（`MainActivity.kt:343-345`），而 `IS_PRO` 恒为 false——**当前发布版实际不叠加传感器 OSD**；免费版反而叠加 "NovaSense Pro" 水印。

### 5.8 构建配置与限制

`build.gradle.kts`：applicationId `com.novasense.agent`，versionName **0.1.0**，compileSdk 36、minSdk 21、targetSdk 35、JDK 17。依赖 Compose BOM 2025.01.00 + Material3、NanoHTTPD 2.3.1、DataStore、lifecycle-service。**无任何 productFlavors/sourceSets**——`app/src/modern/` 源码集实际未参与构建，是**死代码**（与 main 的差异仅是 OSD 绘制方式的修复分支）。`gradle.properties` 硬编码 Linux JDK 路径，Windows 本机需覆盖。

**与 Gateway/Cloud 的对接：当前代码零实际对接**——无任何上行推流、注册、心跳或事件回调逻辑，`ENABLE_CLOUD` 常量未被引用。白皮书将其定位为"规划中的接入面"。

---

## 6. NovaSense Server Plugin — 云平台插件

`novasense-server-plugin/` 是挂载在 EasyKai/VeroRun 平台下的一个 Flask 插件（共 6 文件，约 941 行），是 Gateway 的**多租户 Web 视图 + HTTP 代理**。

### 6.1 文件结构

| 文件 | 行数 | 说明 |
|------|------|------|
| `plugin.json` | 60 | 插件清单 |
| `__init__.py` | 434 | 唯一入口：全部路由集中在单个 Blueprint |
| `templates/novasense/index.html` | 339 | 设备墙 |
| `templates/novasense/settings.html` | 32 | 租户绑定页 |
| `i18n/en.yml`、`zh-CN.yml` | 各 38 | 翻译文件（实际未被消费，见 §13） |

> **纠偏**：`NOVASENSE_PLAN.md` 与 README 设想的 `routes/`、`static/`（ns-tokens.css、PWA manifest）目录**不存在**；Blueprint 已声明 `static_folder='static'` 但目录未建（`__init__.py:120-126`）。

### 6.2 plugin.json 与挂载

- identifier `novasense`，version 0.1.0，作者 EasyKai，`min_app_version: 0.10.0`，category=system，tags=[监控,感知,IoT,摄像头]。
- 内置 config：`gateway_host: http://203.0.113.17:8899`（Tailscale 内网 IP）、`gateway_password: admin`、`refresh_interval: 5`。
- **权限声明仅 `["network.request"]` 一项**——无 routes/scheduler/dag/health/events 申报，`hooks` 与 `agents` 均为空数组。
- **挂载前缀 `/plugin/novasense/`**：前端 JS 硬编码 `const API = '/plugin/novasense/api'`（`index.html:237`），与规划一致（`NOVASENSE_PLAN.md:27`）。

### 6.3 端点与通信方式

共 10 个端点（`__init__.py`），全部为**代理或本地读写**：`/`、`/settings`（渲染页面）、`/api/i18n`（12 个硬编码翻译键）、`/api/devices`、`/api/health`（透传 Gateway）、`/api/proxy/start/<id>`（让 Gateway 起流并返回 `hls_url`）、`/api/camera/<id>/settings`（透传）、`/api/tenant/config`（读写本用户 gateway_host/password/hls_base）、`/api/snapshots`、`/api/snapshot/take/<id>`（调 Gateway 取 base64 JPEG 落盘用户目录）。

模块导入时即执行 `init_tables()`（`__init__.py:65` 副作用），在 `../../data/easykai.db` 建两张表：`novasense_gateways`（user_id 主键，多租户 Gateway 绑定）与 `novasense_snapshots`（按用户隔离的抓拍）。

**通信方式**：纯 **HTTP REST 代理**——`GatewayClient`（requests.Session）先 `POST /api/login {password}` 换 Cookie，60 秒过期或 401 时重登，再透传各请求。**插件不直接触碰 RTSP/RTMP/JPEG 流**；视频播放走 MediaMTX 的 HLS（`hls_base` 默认 `http://localhost:8888`，接入方式为"SSH 反向隧道 localhost:18899"）。

**没有设备 CRUD（增删）、没有 RTMP 转发、没有 AI 分析、没有能力矩阵上报**——设备均在 Gateway 侧管理，插件是只读视图 + 代理。这与 README §Cloud 声称的"CRUD、RTMP 推流转发、AI 画面分析、能力知识库、订阅计费、PWA APP"存在显著差距（详见 §12）。

### 6.4 权限门控（不完备）

仅 `/api/tenant/config`、`/api/snapshots`、`/api/snapshot/take` 三个端点校验 `_get_current_user_id()`（解析 Bearer JWT）并返回 401；而 `/api/devices`、`/api/health`、`/api/proxy/start`、`/api/camera/settings` 在未登录（user_id=0）时**静默回退默认租户** `http://localhost:18899/admin` 继续代理（`__init__.py:217-226`），存在**跨租户读取**与 **gateway_password 明文存储**风险。

---

## 7. NovaSense Brand — 品牌视觉系统

`novasense-brand/`（12 文件，约 1500 行）是产品的设计资产，统一"感知星座"隐喻。

### 7.1 设计令牌（`tokens.css`，61 行，Design Tokens v1.0）

- **背景四级**：深空黑 `#0A0E14` → `#0A121A` → `#111B25` → `#141E2A`。
- **功能色**：星芒青蓝 `#00C8FF`（主）、星云紫 `#7C4DFF`（辅）、荧绿 `#69F0AE`（在线）、琥珀 `#FFAB40`（警告）、星尘红 `#EF5350`（错误）、星际灰 `#5A7288`。
- 文字三级 + 禁用级、边框三级、字体三族（Inter display / 系统 body / JetBrains Mono code）、间距 4–32px 六级、圆角、青蓝辉光阴影 `--ns-shadow-glow`、统一 150ms 过渡、主渐变青→紫 135°。

令牌曾以副本形式散落于 Gateway 前端（`frontend/ns-tokens.css`）与插件页面内联；2026-09-19 冗余清理已删除无引用的前端副本，`brand/tokens.css` 为唯一事实源，插件页面继续以内联方式复用同一套值。

### 7.2 四产品图标与差异化隐喻

全部青/紫渐变 + 高斯模糊辉光（`assets/*.svg`）：Gateway=中心星核+三层虚线轨道环（枢纽）；Agent=中心星+八向光芒+扩散波环（新星）；Cloud=中心枢纽连接 6 颗异色星点成星座（全局）；Viewer=镜头圈+十字准星+四角取景框（观星镜）。另有 `loading-animation.svg`（186 行）：10 节点错峰呼吸、四轨差速旋转、彗星划过。

### 7.3 界面预览与架构全景

`brand-board.html`（377 行）定义色板/渐变/图标/UI 组件规范/字体/品牌故事。三端 `preview-*.html` 给出桌面 Web（Gateway）、Android 手机壳（Agent）、PWA 400px（Cloud）的高保真预览——其中 Agent 预览含 3840×2160 分辨率、H.264/H.265、传感器 Tab 等**超出当前实现的愿景**。`architecture.html` 的三层系统全景见 §3.1。

---

## 8. NovaSense Viewer — 远程查看器（预留）

`novasense-viewer/` 经 `find` 核实为**空目录**（0 文件）。与 brand 预览中 "Viewer=移动端/观星镜" 的定位对应，目前仅是规划位（`NOVASENSE_PLAN.md:16`、`NOVA-VISUAL-PLAN.md:20` 描述其应有功能：设备列表→选择→HLS 全屏播放、多设备滑动切换、画中画、推送通知）。尚无代码。

---

## 9. 跨组件事件与告警机制

Gateway 的实时事件走 **SSE**（`/api/events`，channel 广播，慢客户端丢弃，`main.go:1937-1992`）→ 前端 `EventSource` + Web Notification（`index.html:1352-1386`）。完整告警链路：

```
scene 运动检测 → 抓拍 → pigo 检脸 → motion/face_events 入库
   → SSE 广播 → 浏览器通知
   → (可选) VPS /api/client/analyze 云端 AI 判读（base64 快照 + X-API-Key，main.go:2299）
   → 运动触发录像 + 冷却停录（main.go:1408-1427）
```

> 注意：`/api/events` 与 `/hls` 位于免鉴权白名单——事件流可被匿名订阅（见 §13）。Agent 侧的运动检测目前只在本机 `/status` 呈现，尚无向 Gateway 的事件上行通道。

---

## 10. 安全模型与鉴权

三组件采用**各自独立、面向局域网/可信环境**的轻量鉴权，均非面向公网设计：

| 组件 | 鉴权方式 | 关键实现与风险 |
|------|----------|----------------|
| Gateway | 内存 session（24h 滑动过期）+ Cookie；`ADMIN_PASSWORD`（默认 `admin`，启动告警） | `auth.go:77`；会话仅内存（重启全登出）；CORS `*`；明文密码存库、URL 内嵌凭证（`store.go`、`proxy.go:59-68`） |
| Agent | 无（依赖局域网隔离）；License 为离线空壳 | `usesCleartextTraffic="true"` 供局域网 HTTP |
| Server Plugin | 部分端点 Bearer JWT；未登录静默回退默认租户 | `gateway_password` SQLite 明文存储；跨租户读取风险 |

产品整体定位是"旧手机 + 局域网 + 浏览器"的自托管感知网络，公网暴露需自行叠加反代/隧道（插件文档即建议 SSH 反向隧道）。

---

## 11. 部署形态

**Gateway（三种）**：
- **Docker Compose**（host 网络，三容器：mediamtx / nms-backend / 可选 ruview）；前端只读挂载热更新；Dockerfile 双阶段 glibc（CGO 链 XM SDK）+ ffmpeg + wapa-pull，`EXPOSE 8899`（`docker-compose.yml`、`Dockerfile:31`）。
- **systemd**：`deploy/setup.sh [systemd|docker]`，两个 unit（`Restart=always`、`NoNewPrivileges`/`ProtectSystem`）。
- **裸机**：`backend/run.sh`（写死个人路径 `/home/deployuser`，日志到 `/tmp`）。
- 环境变量：`DATA_DIR`、`ADMIN_PASSWORD`、`VPS_PLUGIN_URL`/`VPS_API_KEY`、`FRONTEND_DIR`。

**Agent**：`./gradlew assembleDebug` 编译或安装现成 APK（`README.md:185-187`）。

**Server Plugin**：将 `novasense-server-plugin/` 部署到 EasyKai 平台 `plugins/` 目录（`README.md:190-191`），依赖 `../../data/easykai.db`。

---

## 12. 现状评估：README 声称 vs 代码实际

本白皮书以"第三方交付标准"逐条核对文档声称与真实代码。区分**缺陷**与**未覆盖范围（不算 bug）**。

### 12.1 Gateway — 大部分能力属实

| 声称 | 核实 | 证据 |
|------|------|------|
| RTSP/ONVIF/HTTP MJPEG/USB 接入 | ✅ 属实 | `discovery.go`、`proxy.go` |
| FFmpeg 代理 + 自动重连(3s) | ✅ 属实（指数退避 3s→60s + 卡死自愈） | `proxy.go:401-424` |
| HLS(3-5s) + WebRTC(WHEP<1s，自动降级) | ✅ 属实 | `index.html:873-883` |
| PTZ 云台 | ⚠️ 部分：仅海康 ISAPI+CGI，**非 ONVIF 标准** PTZ | `main.go:1045-1171` |
| 导播台 RTMP 推流到第三方 | ⚠️ 无独立"抖音/B站推流任务"模块，实际=导播台任意 RTMP push URL | `director.go`、`proxy.go:70`（空注释） |
| 运动/人脸/对讲/存储/Docker/许可证 | ✅ 属实 | 见 §4 |
| 双向对讲 | ⚠️ 仅"下行到摄像头"通路，未见摄像头音频回流浏览器 | `main.go:1896-1905` |
| （未写进 README 的隐藏能力） | ➕ WAPA 私有协议、雄迈 SDK、鱼眼去畸变、SRT/MoQ | `cmd/wapa-pull`、`sdk_xm.go`、`panorama.html` |

### 12.2 Agent — 多能力"骨架完整但未接通"

| 声称（README.md:64-89） | 核实 | 证据 |
|------|------|------|
| MJPEG 浏览器直接看 | ✅ 属实 | `HttpServer.kt:202-207` |
| **RTSP H.264（VLC/FFmpeg 可拉）** | ❌ **编码器 Surface 未接像素源，实际不出流** | `H264Encoder.kt`（getInputSurface 无调用者） |
| AAC 实时音频（HTTP + RTSP 双路） | ❌ HTTP `/audio.aac` 是桩（返回 -1）；RTSP 视频断链 | `HttpServer.kt:219-241` |
| **传感器数据叠加 OSD** | ❌ 被 `IS_PRO=false` 关闭，发布版不叠加 | `MainActivity.kt:343-345` |
| 变焦/镜像/切换（1x-8x） | ✅ 属实（入队帧回调应用） | `MainActivity.kt:192-255` |
| 曝光/白平衡 API | ⚠️ 入队但无应用分支，静默丢弃 | `HttpServer.kt:368-379` |
| 网格运动检测(32×24) | ✅ 属实（仅瞬时布尔，无事件历史） | `MainActivity.kt:382-428` |
| 多分辨率 320×240–3840×2160 | ⚠️ 受 Camera1 supportedPreviewSizes 约束 | `MainActivity.kt:257-307` |
| 完整 REST API | ⚠️ `/api/settings` 为 stub | `HttpServer.kt:292-302` |

### 12.3 Server Plugin — 云组件实际形态远小于命名

README "NovaSense Cloud" 章节（`README.md:92-116`）声称 CRUD、RTMP 推流转发、HLS 直播、AI 画面分析、手机远程控制、设备能力知识库、订阅计费、PWA APP。实际 `novasense-server-plugin/` **仅实现设备只读视图 + Gateway HTTP 代理 + 租户绑定 + 抓拍落盘**；无设备增删、无 RTMP 转发、无 AI 分析、无能力矩阵上报、无订阅计费、无独立 PWA manifest。它是 README 所述 Cloud 的一个早期子集。

### 12.4 版本口径（曾不一致，已统一为 v0.8.4）

历史上三组件版本口径混乱：Gateway 代码 0.8.2 / git tag v0.5.0 / 品牌 v0.1.0；Agent versionName 0.1.0 而 ComposeUI/启动页遗留 "v4.0"；插件 0.1.0。**2026-09-19 已将全部版本载体统一为 0.8.4**（Gateway main.go×3+启动横幅、login.html、Dockerfile、docker-compose.yml、README；Agent build.gradle.kts versionCode=84/versionName=0.8.4、main 与 modern 页脚、activity_splash.xml；插件 plugin.json）。遗留未动项：Gateway 二进制名仍为 `video-stream-manager`（属命名问题非版本问题）、启动横幅产品旧名、git tag 未补打 `v0.8.4`（留待开发者提交时执行）、`main.go` 中不存在 `main.version` 变量故 README 的 `-ldflags -X main.version` 示例实际无效。

---

## 13. 技术债务清单

供工程规划参考（非"缺陷"，属"已识别的技术债"）：

**Gateway**
- 单体拆分半途：`.split_needs_work`、`main.go.bak`、healthCheck 仍在 `main.go:949-1041`。
- 硬编码 LAN IP `192.0.2.107`（`main.go:576,731`、`index.html:976`、`phone.html:42`）；`run.sh` 残留 `/home/deployuser` 个人路径。
- 明文密码存库 + URL 内嵌凭证；会话仅内存 + 默认密码 admin + `/hls`、`/api/events` 免鉴权 + CORS `*`。
- SSE 广播无背压；SQLite `SetMaxOpenConns(1)` 限并发；aHash 人脸表征弱（无特征向量，光照敏感）；`ImportAll` 清表式导入有丢数据风险。
- 手动录制同步阻塞 HTTP。

**Agent**
- RTSP 视频链路未接通、`/audio.aac` 桩、`/api/settings` stub、exposure/white_balance 入队即弃。
- 坚持 Camera1（minSdk 21 兼容旧机是动因，但 API 已废弃）；OSD/镜像/水印多次 JPEG 编解码往返，高分辨率高帧率 CPU 压力大；运动检测每帧全量数组分配易触发 GC；RTSP 单客户端、无 RTCP、时间戳固定步进。
- `src/modern/` 死源码集；授权体系空壳；`gradle.properties` 硬编码 Linux JDK 路径。

**Server Plugin**
- 权限门控不完备（4 个端点未登录静默回退默认租户）；`gateway_password` 明文存储；i18n yml 文件实际未被消费（`/api/i18n` 返回扁平硬编码键，命名不一致）；`static/` 目录声明了但未建；设置按钮仅 `alert("coming soon")`。

### 13.4 清理记录（2026-09-19，monorepo）

经"属实即删"复核，以下**确证级**项已在单仓中清除（共 3 个 refactor 提交，净删 ≈9100 行 + 729KB 无引用图片）：Agent `src/modern/` 死源码集（1401 行）、零调用 `ComposeUI.kt`、`SensorCollector.sensorDataFlow` 冗余 callbackFlow、孤儿布局 `activity_splash.xml`（v0.8.4 页脚此前误写入该孤儿文件，现移除）、Gateway 不参与编译的 `sdk/xm_sdk_bridge.c`+`sdk/include/`（7185 行，保留必需的 `libxmnetsdk.so`）、`.split_needs_work` 标记、无引用前端资产（`ns-tokens.css` 副本、`logo-novasense.svg`、4 个 icon/splash PNG）、前端 schedule 死块（tab+modal+JS ≈145 行）与 `switchTab` 双实现合并（顺带修复侧栏高亮失效）、`run.sh` 个人路径重写为可移植版、`go mod tidy` 移除 gocv 并修正 pigo/websocket 标注、插件空 `static_folder` 声明。白皮书正文保留清理前的观察原文以维持审计基线。**疑似项未动**（等待产品决策）：`phone.html`、`ruview-ui/viz.html`/`tests/`/`mobile/` 子工程、`i18n/*.yml`、双份 `auto.crt`、`images/手机*.png`、`main.go.bak`（未入库）。安全类条目（明文密码/硬编码 IP/门控缺口）不受本次清理影响，仍待处理。

---

## 14. 结论

NovaSense 是一条**以本地边缘网关为绝对核心**的自托管感知网络产品线。其真实工程重心与技术成色集中在 `novasense-gateway/`——一个把异构摄像头/传感器统一为多协议可分发流、并附带运动检测、人脸识别、导播推流、双向对讲、8 阶段设备发现、多厂商 SDK/私有协议旁路的成熟 Go 单进程服务。它的零重型依赖架构（纯 Go SQLite + exec FFmpeg + MediaMTX）、FFmpeg 进程生命周期工程化、以及把鱼眼矫正放在浏览器侧、把 RuView ticket 铸票放在服务侧等设计，体现了清晰的边缘优先工程哲学。

`novasense-agent/` 是完成度次之的采集端，MJPEG 与本地控制链路可用，但 RTSP/H.264/音频/传感器 OSD 等 README 亮点尚处"骨架完整、连线未通"状态，需一轮接通与端到端验证。`novasense-server-plugin/` 是刚纳入版本管理之外的云侧早期子集，与 README 所述 "Cloud" 有明确差距。`novasense-brand/` 提供了跨端一致的高质量设计令牌系统，`novasense-viewer/` 尚为规划空位。

整体判断：**架构方向清晰、Gateway 底座扎实、品牌系统成熟；产品叙事（README/brand 愿景）领先于代码实现**，尤其云组件与手机端若干媒体链路。对第三方开发者或投资决策者，建议以本白皮书 §12 的"声称 vs 实际"表作为基线，优先补齐 Agent 的 RTSP/音频/OSD 接通、Server Plugin 的门控与 CRUD、以及 Gateway 的单体拆分与安全收尾。

---

## 附录：关键源码索引

### A. Gateway 完整路由（`backend/main.go:133-185` + `hls.go`）

| 方法 | 路径 | 功能 |
|------|------|------|
| GET/POST | `/api/devices`；GET/PUT/DELETE `/api/devices/{id}` | 设备列表/添加/详情/改名分组/删除 |
| GET/POST/DELETE | `/api/recordings` | 录像列表/手动录制(同步阻塞)/删除 |
| GET/POST；GET/PUT/DELETE | `/api/schedules[/{id}]` | 定时录制计划 |
| GET/POST；PUT/DELETE；GET | `/api/snapshots[/list/{id}]` | 抓拍配置与历史 |
| POST | `/api/proxy/start|stop/{id}`；`/api/proxy/config/{id}` | FFmpeg 代理启停/改分辨率 |
| GET | `/api/status`、`/api/health`、`/api/metrics` | 状态/健康/代理指标 |
| GET/PUT | `/api/export`、`/api/import` | JSON 备份导入导出 |
| GET/PUT | `/api/camera/{id}/settings` | 图像参数(SDK/ONVIF/CGI) |
| POST | `/api/ptz/{id}/{action}`；`/stop`；`/preset/{n}` | 云台(ISAPI+CGI) |
| GET/POST | `/api/motion/config[/{id}]`；GET `/api/motion/events/*` | 运动配置与事件 |
| GET/POST | `/api/director*` | 导播/切流/overlay/外推 |
| GET/PUT | `/api/storage` | 存储配置/磁盘统计/清理 |
| GET | `/api/events` (SSE) | 实时事件流 |
| WS | `/api/talk/{id}` | 对讲 PCM→AAC→RTMP |
| GET/PUT | `/api/groups` | 分组 |
| GET/POST/PUT/DELETE | `/api/faces*`；GET `/api/face-events*` | 人脸库与识别事件 |
| GET/PUT | `/api/v4l2/{id}` | USB 摄像头控件 |
| ANY/POST | `/api/phone/{id}/{cmd}`；`/api/phone/frame/{id}` | 手机 API 透传/帧上行 |
| GET/POST | `/api/discover*` | 设备发现五连 |
| POST/GET | `/api/license/*` | 授权 |
| GET/WS | `/api/ruview/status`、`/api/ruview/ws` | RuView 桥 |
| GET | `/hls/*`、`/videos/*`、`/snapshots/*`、`/faces/*`、`/ui/*` | 流与文件静态服务 |

### B. 数据模型（Gateway，`store.go:112-215` 迁移）
`devices`（id/protocol rtsp|http|usb|wapa/url/账号/status/group_name/capability）、`recordings`、`schedules`、`snapshot_configs`、`motion_configs`、`motion_events`、`licenses`、`faces`（label/face_hash/thumb/seen_count）、`face_events`（motion_event_id/confidence/score/bounds）、`retention_config`。文件层：`videos/{deviceId}/{YYYYMMDD}/*.mp4`、`snapshots/{configId}/*.jpg`、`faces/face_*.jpg`。

### C. 端口总表

| 端口 | 组件/协议 |
|------|-----------|
| 8899 | Gateway HTTP（Go 后端 + 前端 SPA） |
| 8888 / 1935 / 8554 / 8889 / 8890 / 8892 | MediaMTX：HLS / RTMP / RTSP / WebRTC / SRT / MoQ |
| 8189 | WebRTC ICE UDP |
| 8080 / 8554 | Agent：MJPEG+REST / RTSP |
| 3000 / 3001 | RuView：HTTP / WebSocket |
| 18899 | 插件反向隧道至 Gateway（SSH） |

### D. 组件规模统计
Gateway 后端 ≈7044 行 Go（+ cmd/wapa-pull 132 行 + C 桥 + 239KB 分类器）；Agent ≈2977 行主 Kotlin（+1401 行 modern 死代码）；Server Plugin ≈941 行（Python+HTML+YML）；Brand ≈1500 行设计资产；Viewer 0。

---

*本白皮书基于 `F:\projects\novasense` 各目录源码逐文件核实撰写，"声称 vs 实际"结论以代码为准，来源路径均可复核。*
