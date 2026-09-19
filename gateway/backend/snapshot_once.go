package main

// snapshot_once.go — FIX-11: 单帧抓拍端点 POST /api/snapshots/once/{deviceID}
//
// 背景: 插件 api_snapshot_take 调用的 GET /api/snapshot/{id}(单数)从未存在(契约断裂)。
// 网关侧补最小新增(方案 B), 插件改为消费本端点(方案 A 的调用方修正), 端到端闭环。
// 返回 {image: base64(jpeg)} 与插件既有读法兼容。

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (s *Server) handleSnapshotOnce(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/snapshots/once/")
	deviceID = strings.TrimRight(deviceID, "/")
	device := s.store.GetDevice(deviceID)
	if device == nil || len(deviceID) < 8 {
		s.error(w, "device not found", 404)
		return
	}

	fp := filepath.Join(s.dataDir, "snapshots",
		fmt.Sprintf("once_%s_%d.jpg", deviceID[:8], time.Now().UnixMilli()))
	os.MkdirAll(filepath.Dir(fp), 0755)

	inputURL := buildInputURL(device)
	args := []string{"-y", "-loglevel", "error"}
	if strings.HasPrefix(inputURL, "rtsp://") || strings.HasPrefix(inputURL, "rtsps://") {
		args = append(args, "-rtsp_transport", "tcp")
	}
	args = append(args, "-i", inputURL, "-vframes", "1", "-q:v", "2", fp)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
		msg := string(out)
		if len(msg) > 200 {
			msg = msg[len(msg)-200:]
		}
		log.Printf("[snap-once] %s failed: %v (%s)", deviceID[:8], err, msg)
		os.Remove(fp)
		s.error(w, "capture failed: device unreachable or stream not ready", 502)
		return
	}
	data, err := os.ReadFile(fp)
	os.Remove(fp) // 一次性抓拍不落库, 直接回传
	if err != nil || len(data) < 100 {
		s.error(w, "capture produced no image", 502)
		return
	}

	s.json(w, map[string]interface{}{
		"device_id":   deviceID,
		"captured_at": time.Now().Format(time.RFC3339),
		"image":       base64.StdEncoding.EncodeToString(data),
	})
}
