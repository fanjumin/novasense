# V4L2 摄像头控制 API 方案

## 目标
为 USB 摄像头（V4L2 设备）增加 Web API，让前端能调节亮度、对比度、饱和度等参数。

## 改动范围（单文件）

**`backend/main.go`** — 新增：

### API 路由
| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/api/v4l2/{device_id}` | 列出该设备所有可用 V4L2 控制项（名称、范围、当前值） |
| PUT | `/api/v4l2/{device_id}` | 设置指定控制项的值 |

### 实现方式
- 通过 `v4l2-ctl -d <dev> --list-ctrls` 获取控制列表（解析输出）
- 通过 `v4l2-ctl -d <dev> --set-ctrl <name>=<value>` 设置值
- 从 `Device.URL` 获取 `/dev/video*` 路径
- 只对 `protocol=="usb"` 的设备生效，其他返回 400

### PUT 请求体
```json
{
  "control": "brightness",
  "value": 10
}
```

### 返回示例（GET）
```json
{
  "device": "/dev/video0",
  "controls": [
    {"name": "brightness", "type": "int", "min": -64, "max": 64, "step": 1, "default": 0, "value": 0},
    {"name": "white_balance_automatic", "type": "bool", "default": true, "value": true},
    ...
  ]
}
```

## 验证
```bash
# 列出 USB 摄像头控制项
curl http://localhost:8899/api/v4l2/51097157

# 设置亮度
curl -X PUT http://localhost:8899/api/v4l2/51097157 \
  -H "Content-Type: application/json" \
  -d '{"control":"brightness","value":20}'
```
