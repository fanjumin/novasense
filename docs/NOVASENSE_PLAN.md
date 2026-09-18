# NovaSense 产品线统一规划

## 品牌线

```
NovaSense                    ← 总品牌
├── NovaSense Cloud          ← EasyKai 插件 (管理后台)
├── NovaSense Gateway        ← 本地边缘服务器 (Go)
└── NovaSense Agent          ← 手机端 APP (Android)
```

## 三个代码仓库

| 旧名称 | 新仓库名 | 新代码标识 | 说明 |
|--------|---------|-----------|------|
| `easykai-stream-monitor` | `novasense-cloud` | plugins/novasense/ | EasyKai 管理插件 |
| `video-stream-manager` | `novasense-gateway` | 整个项目改门面 | 本地 Go 网关 |
| `netcam-android` | `novasense-agent` | app 包名 → com.novasense.agent | 手机端采集 APP |

## 改动详情

### 一、NovaSense Cloud (当前 stream_monitor 插件)

- **仓库:** github.com/fanjumin/**novasense-cloud**
- **目录名:** `plugins/novasense/`
- **类名:** `NovaSensePlugin`
- **路由前缀:** `/plugin/novasense/`
- **plugin.json:**

```json
{
  "name": "novasense",
  "version": "0.1.0",
  "description": "NovaSense 感知网络 — 设备管理、实时可视化、AI分析",
  "author": "EasyKai",
  ...
}
```

### 二、NovaSense Gateway (当前 video-stream-manager)

- **仓库:** github.com/fanjumin/**novasense-gateway**
- **门面重命名:**
  - 目录 `backend/` → 不变
  - Go 模块名 `video-stream-manager` → `novasense-gateway`
  - 可执行文件名 `video-stream-manager` → `novasense-gateway`
  - 二进制/脚本里所有 `video-stream-manager` 引用改为 `novasense-gateway`
  - Docker 镜像名改为 `novasense-gateway`
- **README/frontend 中"视频流管理平台" → "NovaSense 网关"**
- **版本 v0.8.3 → v0.1.0**（产品重置）

### 三、NovaSense Agent (当前 netcam-android)

- **仓库:** github.com/fanjumin/**novasense-agent**
- **包名:** `com.netcam` → `com.novasense.agent`
  - app/src/main/java/com/netcam/ → app/src/main/java/com/novasense/agent/
  - AndroidManifest.xml 中 package 名
  - build.gradle.kts 中 applicationId
- **APP 名称:** "NetCam" → "NovaSense"
- **版本:** 0.8.1 → 0.1.0（产品重置）
- **图标/启动画面:** 需要新设计

## 视觉方向

- 主色: 深空蓝 (#0a0e14) + 紫色渐变 → 星爆意象
- Logo: 星座/星点连线 + 透镜元素
- 风格: 科技感、SaaS、B端专业

## 实施顺序

1. 先改 **NovaSense Cloud**（插件，影响最小，方便测试）
2. 再改 **NovaSense Agent**（手机端，包名变更需完整测试）
3. 最后改 **NovaSense Gateway**（网关，涉及运行时换名）
