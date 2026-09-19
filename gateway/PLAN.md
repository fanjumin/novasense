# VSM → easykai.cn 集成方案

## 架构

```
用户浏览器/APP
    │
    ▼
easykai.cn (VPS 100.124.0.103, Flask :8081)
    │  JWT 认证
    │  ┌─────────────────┐
    │  │ plugin/vsm/*     │  ← Python 插件
    │  │ - 设备列表/管理   │
    │  │ - 直播画面嵌入    │
    │  │ - 远程控制API     │
    │  └──────┬──────────┘
    │         │ HTTP 代理
    ▼         ▼
本地 VSM (<网关IP>:8899)
    │  Go backend
    ├── FFmpeg 代理
    ├── MediaMTX (HLS)
    └── SQLite 设备库
```

## 步骤

### 1. VSM 插件（Python）

放在 easykai.cn 的 `plugins/video_stream_manager/` 下：

- `plugin.json` — 元数据声明
- `__init__.py` — `VSMPlugin(BasePlugin)` 插件类
- `routes.py` — Flask Blueprint，代理 VSM API + 嵌入直播页面
- `templates/` — 管理界面 HTML（设备管理、直播看板）
- `i18n/zh-CN.yml` — 中文翻译
- `i18n/en.yml` — 英文翻译

插件注册路由 `/plugin/vsm/`，通过 HTTP 调本地 VSM API：
- `GET /plugin/vsm/devices` → `http://127.0.0.1:8899/api/devices`
- `POST /plugin/vms/proxy/start/<id>` → 启动代理
- `POST /plugin/vsm/settings/<id>` → 远程设置（转发到手机 HTTP API）

页面用 hls.js 播放 HLS 流。

### 2. 远程 APP（PWA）

写一个可安装的 PWA，放在 easykai.cn 的插件静态目录下：
- `manifest.json` — 安装配置（图标、名称）
- `sw.js` — Service Worker（离线缓存）
- `index.html` — 全屏摄像头网格 + 设备管理入口

APP 通过 easykai.cn 的 JWT Token 认证，插件代理到本地 VSM。

## 依赖

- easykai.cn 的 `plugins/base.py` 插件框架
- hls.js（浏览器 HLS 播放）
- VSM 运行在本地 8899 端口
- easykai.cn 需要能访问到本地 VSM
