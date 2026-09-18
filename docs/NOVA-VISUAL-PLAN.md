# NovaSense 视觉系统规划

---

## 一、品牌核心理念

**NovaSense** = 感知新星。每一台旧手机、USB 摄像头、传感器，都是暗夜中的一颗星。被感知网络点亮，就爆发成一颗新星（Nova）。当几百颗连在一起，就构成了一个「感知星座」。

> 不是监控系统，是感知层。

---

## 二、四个产品名称与定位

| 产品 | 角色 | 视觉隐喻 | 环境 |
|------|------|----------|------|
| **NovaSense Gateway** | 本地网关 | 星座的"中心枢纽"——引力点 | 一台 Linux 主机，静默运行 |
| **NovaSense Agent** | Android 摄像头源端 | 单颗"点亮的新星"——感知的起点 | 旧手机、旧平板，放在角落 |
| **NovaSense Cloud** | 云平台 | 星座的"星图"——全局视角 | Web 后台，SaaS 仪表盘 |
| **NovaSense Viewer** | 远程查看 App | 携带在手的"观星镜" | 手机 App，随时随地 |

---

## 三、色彩系统

### 主色调：深空暗色系

```
品牌底色   #0A0E14    深空黑（已用于 VSM 前端，保持一致）
品牌主色   #00C8FF    星芒青蓝（科技感、发光感）
品牌辅色   #7C4DFF    星云紫（氛围、深邃、神秘感）
```

### 语义色

```
在线/活跃   #69F0AE    荧绿（星座中亮起的星）
离线/警告   #FFAB40    琥珀（暗星闪烁）
错误/危险   #EF5350    星尘红
信息/中性   #5A7288    星际灰
```

### 渐变

```
主渐变     #00C8FF → #7C4DFF    青蓝→星云紫（Logo、卡片高亮、C 端触点）
氛围渐变   #0A0E14 → #111B25    深空微亮（背景、卡片）
```

### 色值比例

```
底色（深空）   60%    背景、大面积
青色（星芒）   15%    主交互、品牌高亮
紫色（星云）   10%    辅助强调、氛围
灰色（中性）   10%    文字、次要信息
语义色（点缀）  5%    状态指示
```

---

## 四、Logo/品牌图形

### 主 Logo 概念：星座 + 新星爆发

```
        ★
       / \
      /   \
     ★—————★
      \   /
       \ /
        ★
```

1. **简化星形** — 4-6 个节点连成星座图案，核心一颗星最大（代表 "Nova" 爆发）
2. **圆形星轨** — 环绕的轨道/星环，代表感知网络的"连接"属性
3. **三芒星** — 三个尖角的星芒，代表三要素：视觉、环境感知、未来扩展

### 四个产品的差异化图标

| 产品 | 图标概念 | 形态 |
|------|---------|------|
| Gateway | 实心星 + 环 | 一颗星被同心环绕（中心/枢纽） |
| Agent | 发光星 | 单颗星，射线向外（发散/感知） |
| Cloud | 星群/星图 | 多颗星连线成网（聚合/全局） |
| Viewer | 星 + 眼睛/透镜 | 星在镜片中心（观看/发现） |

四个图标共用统一风格：**细线几何 + 星节点 + 连接线**，可识别为同一家族。

---

## 五、字体系统

### 品牌标题字体

```
Inter (Google Fonts)
- Weight 600/700 用于大标题
- 无衬线、几何感、科技、高可读性
```

### UI 正文字体

```
系统字栈 (跨平台一致)
font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI',
             Roboto, 'Helvetica Neue', Arial, sans-serif;
```

### 代码/技术展示（面向开发者）

```
JetBrains Mono / SF Mono
- 配置面板、API 文档、技术指标
```

---

## 六、UI 风格指引

### 通用原则

- **暗色主题** — 深空底色，信息像星辰一样浮在暗色上
- **薄边界** — 1px `rgba(255,255,255,0.06)` 分隔线
- **辉光效果** — 青色 hover 时微发光 `box-shadow: 0 0 8px rgba(0,200,255,0.2)`
- **圆角** — 小控件 4px，卡片 8px，大模态 12px
- **透明度层级** — 主要信息 100%，次要 70%，辅助 40%

### 组件风格

```
按钮:
  主按钮: 青色实底 #00C8FF + 白色文字
  次按钮: 透明 + 1px 青色边框
  危险: 红底
  禁用: 灰色 0.3 透明度

卡片:
  深色底 #0A121A + 1px #1A2A3A 边框
  hover: 边框变青 0 0 6px 辉光

输入框:
  深色底 #0D151F + 1px #1E2D3D 边框
  focus: 青色边框

标签/徽章:
  在线: 绿底绿字
  离线: 红底红字
  推流中: 青底青字
```

---

## 七、图片与插画风格

### 需要制作的素材清单

```
hero/（品牌封面图）
├── novasense-constellation.jpg     星座感知网络概念图（深空 + 节点连线）
├── novasense-devices-family.jpg    四设备同框（网关+旧手机+云+手机查看）

icons/（四个产品图标）
├── icon-gateway.svg    中心星 + 环
├── icon-agent.svg      发光单星
├── icon-cloud.svg      星群连线
├── icon-viewer.svg     星 + 透镜

patterns/（背景纹理）
├── dot-grid.svg        点阵网格（深空背景底纹）
├── constellation-lines.svg  星座连线纹理（极淡，用于大面积背景）

illustrations/（插画场景）
├── illu-retail.svg      零售店场景（货架 + 传感器节点 + 光点）
├── illu-warehouse.svg   仓库场景
├── illu-home.svg        家庭场景
├── illu-constellation.svg  抽象星座图（品牌故事用）

ui/（UI 素材）
├── nova-loading.svg     加载动画（星旋转/脉冲）
├── nova-empty.svg       空状态（一颗暗星等待点亮）
├── nova-error.svg       错误状态（星碎/连线断）
├── nova-avatar.svg      默认头像（星座风格头像）
```

### 插画风格定义

```
AI 生成提示词模板:

"A dark space scene, deep navy background (#0A0E14),
constellation nodes connected by thin glowing cyan (#00C8FF) lines,
each node is a small bright star, purple nebula (#7C4DFF) in the background,
a network of sensors scattered like stars,
sci-fi minimal flat vector style, 2D illustration,
clean lines, no text, wide aspect ratio 16:9"
```

---

## 八、四种产品的 UI 预览规划

### 1. Gateway Web UI（VSM 前端）
- 现状：已有深色主题 `#0A0E14`，基本符合品牌
- 需调整：替换品牌色为 `#00C8FF`，加入星座背景纹理
- 页面：设备网格（4宫格）、录像列表、推流控制、PTZ

### 2. Agent 界面（Android App）
- 现状：已有 Compose UI + Camera1 预览
- 需调整：品牌色替换、顶部 NovaSense Logo、深色相机 UI
- 页面：相机预览、控制面板（闪光灯/切换/分辨率）、设置页、License 状态

### 3. Cloud 后台（EasyKai 插件 PWA）
- 现状：`pwa.html` 42 行极简白板
- 需重做：品牌化 PWA，设备卡片视图，星座背景
- 页面：设备列表 → 推流控制 → 实时观看

### 4. Viewer App（未来 Android 开发）
- 全新设计：深色系，设备列表 → 选择 → HLS 全屏播放
- 功能：多设备滑动切换、画中画、推送通知

---

## 九、Logo 文本规范

```
主标:
NovaSense
N 大写，S 大写，中间无空格

全称:
NovaSense 感知网络

副标:
constellation of perception

标语文案池:
- 点亮感知新星
- 你的感知星座
- 从一台旧手机开始
- Constellation of Perception
```

---

## 十、执行路线

```
Phase 1（规划—当前）
  ✅ 视觉系统规划文档（本文）

Phase 2（品牌核心资产）
  □ 色彩 Token 定义 → CSS 变量文件
  □ Logo 生成 → SVG
  □ 四个产品图标 → SVG
  □ 品牌插画 x3（零售/仓库/家庭场景）

Phase 3（UI 改造）
  □ Gateway UI 品牌化（替换色值、背景、图标）
  □ Agent UI 品牌化（App 内品牌色、顶部 NovaSense 标）
  □ Cloud PWA 重新设计（品牌化 PWA 页面）

Phase 4（扩展）
  □ Viewer 新建 Android 项目 + 品牌 UI
  □ Loading/Empty/Error 状态插画
  □ 加载动画
  □ 品牌落地页（可选）
```
