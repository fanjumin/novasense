package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxConcurrentFFmpeg = 4
	healthCheckInterval = 15 * time.Second
	backoffBase         = 3 * time.Second
	backoffMax          = 60 * time.Second
	sessionDuration     = 24 * time.Hour
)

type Server struct {
	store        *Store
	ffmpeg       *FFmpegManager
	director     *Director
	disco        *DiscoveryManager
	mux          *http.ServeMux
	dataDir      string
	vpsPluginURL string
	vpsAPIKey    string
	faceDetector *FaceDetector
	faceDir      string // directory for face thumbnails
	sseClients   map[chan string]bool
	sseMu        sync.Mutex
	hlsClient    *http.Client // shared client with persistent cookie jar for MediaMTX HLS proxy
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}

	// Auth check — skip public paths
	if !isPublicPath(r.URL.Path) && !isAuthenticated(r) {
		// If it's an API call, return 401 JSON
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "login_url": "/ui/login.html"})
			return
		}
		// For other pages (like root /), redirect to login page
		if r.URL.Path == "/" || !strings.HasPrefix(r.URL.Path, "/ui/") {
			http.Redirect(w, r, "/ui/login.html", 302)
			return
		}
	}

	s.mux.ServeHTTP(w, r)
}

func (s *Server) json(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) error(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) registerRoutes() {
	// Serve frontend FIRST (before catch-all "/")
	// Priority: FRONTEND_DIR env > dataDir/../frontend > ./frontend (CWD)
	frontendDir := os.Getenv("FRONTEND_DIR")
	if frontendDir == "" {
		frontendDir = s.dataDir + "/../frontend"
	}
	// Try multiple paths: env path, dataDir-relative, CWD-relative
	frontendCandidates := []string{frontendDir}
	if os.Getenv("FRONTEND_DIR") == "" {
		frontendCandidates = append(frontendCandidates, "./frontend")
	}
	var frontendAbs string
	for _, candidate := range frontendCandidates {
		if abs, err := filepath.Abs(candidate); err == nil {
			if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
				frontendAbs = abs
				break
			}
		}
	}
	if frontendAbs != "" {
		fs := http.FileServer(http.Dir(frontendAbs))
		s.mux.Handle("/ui/", http.StripPrefix("/ui/", fs))
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.Redirect(w, r, "/ui/login.html", 302)
				return
			}
			fp := filepath.Join(frontendAbs, r.URL.Path)
			if _, err := os.Stat(fp); err == nil {
				http.ServeFile(w, r, fp)
				return
			}
			log.Printf("[catch-all] 404 for %s %s Host=%s", r.Method, r.URL.Path, r.Host)
			s.error(w, "not found", 404)
		})
	} else {
		s.mux.HandleFunc("/", s.handleRoot)
	}

	s.mux.HandleFunc("/api/devices", s.handleDevices)
	s.mux.HandleFunc("/api/devices/", s.handleDeviceByID)
	s.mux.HandleFunc("/api/recordings", s.handleRecordings)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/proxy/start/", s.handleProxyStart)
	s.mux.HandleFunc("/api/proxy/stop/", s.handleProxyStop)
	s.mux.HandleFunc("/api/proxy/config/", s.handleProxyConfig)
	s.mux.HandleFunc("/api/schedules", s.handleSchedules)
	s.mux.HandleFunc("/api/schedules/", s.handleScheduleByID)
	s.mux.HandleFunc("/api/snapshots", s.handleSnapshots)
	s.mux.HandleFunc("/api/snapshots/", s.handleSnapshotByID)
	s.mux.HandleFunc("/api/snapshots/list/", s.handleSnapshotList)
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/metrics", s.handleMetrics)
	s.mux.HandleFunc("/api/export", s.handleExport)
	s.mux.HandleFunc("/api/import", s.handleImport)
	s.mux.HandleFunc("/api/camera/", s.handleCameraSettings)
	s.mux.HandleFunc("/api/ptz/", s.handlePTZ)
	s.mux.HandleFunc("/api/motion/config", s.handleMotionConfig)
	s.mux.HandleFunc("/api/motion/config/", s.handleMotionConfigByDevice)
	s.mux.HandleFunc("/api/motion/events/", s.handleMotionEvents)
	s.mux.HandleFunc("/api/ruview/status", s.handleRuviewStatus)
	s.mux.HandleFunc("/api/ruview/ws", s.handleRuviewWS)
	s.mux.HandleFunc("/api/phone/frame/", s.handlePhoneFrame)
	s.mux.HandleFunc("/api/discover", s.handleDiscover)
	s.mux.HandleFunc("/api/discover/start", s.handleDiscoverStart)
	s.mux.HandleFunc("/api/discover/stop", s.handleDiscoverStop)
	s.mux.HandleFunc("/api/discover/status", s.handleDiscoverStatus)
	s.mux.HandleFunc("/api/discover/import", s.handleDiscoverImport)
	s.mux.HandleFunc("/api/login", s.handleLogin)
	s.mux.HandleFunc("/api/logout", s.handleLogout)
	s.mux.HandleFunc("/api/license/generate", s.handleLicenseGenerate)
	s.mux.HandleFunc("/api/license/check", s.handleLicenseCheck)
	s.mux.HandleFunc("/api/license/bind", s.handleLicenseBind)
	s.mux.HandleFunc("/api/license/list", s.handleLicenseList)
	s.mux.HandleFunc("/api/director", s.handleDirector)
	s.mux.HandleFunc("/api/director/sources", s.handleDirectorSources)
	s.mux.HandleFunc("/api/director/switch", s.handleDirectorSwitch)
	s.mux.HandleFunc("/api/director/overlays", s.handleDirectorOverlays)
	s.mux.HandleFunc("/api/director/stop", s.handleDirectorStop)
	s.mux.HandleFunc("/api/director/push", s.handleDirectorPush)
	s.mux.HandleFunc("/api/storage", s.handleStorage)
	s.mux.HandleFunc("/api/events", s.handleSSE)
	s.mux.HandleFunc("/api/talk/", s.handleTalk)
	s.mux.HandleFunc("/api/groups", s.handleGroups)
	s.mux.HandleFunc("/api/faces", s.handleFaces)
	s.mux.HandleFunc("/api/faces/", s.handleFaceByID)
	s.mux.HandleFunc("/api/face-events", s.handleFaceEvents)
	s.mux.HandleFunc("/api/face-events/", s.handleFaceEventsByMotion)
	s.mux.HandleFunc("/api/v4l2/", s.handleV4L2)
	s.mux.HandleFunc("/api/phone/", s.handlePhoneAPI)
	s.registerHLSProxy()
}

// ============ V4L2 Camera Controls ============

func (s *Server) handleV4L2(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/v4l2/")
	deviceID = strings.TrimRight(deviceID, "/")

	device := s.store.GetDevice(deviceID)
	if device == nil {
		s.error(w, "device not found", 404)
		return
	}
	if device.Protocol != "usb" {
		s.error(w, "not a USB device", 400)
		return
	}
	devPath := device.URL

	switch r.Method {
	case "GET":
		controls, err := listV4L2Controls(devPath)
		if err != nil {
			s.error(w, err.Error(), 500)
			return
		}
		s.json(w, map[string]interface{}{
			"device":   devPath,
			"controls": controls,
		})
	case "PUT":
		var req struct {
			Control string      `json:"control"`
			Value   interface{} `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.error(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		valStr := fmt.Sprint(req.Value)
		// Convert bool to 0/1 for v4l2-ctl
		switch v := req.Value.(type) {
		case bool:
			if v {
				valStr = "1"
			} else {
				valStr = "0"
			}
		}
		if err := setV4L2Control(devPath, req.Control, valStr); err != nil {
			s.error(w, err.Error(), 500)
			return
		}
		s.json(w, map[string]string{"status": "ok"})
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handlePhoneAPI(w http.ResponseWriter, r *http.Request) {
	// /api/phone/{deviceId}/{command}
	path := strings.TrimPrefix(r.URL.Path, "/api/phone/")
	path = strings.TrimRight(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		s.error(w, "need deviceId/command", 400)
		return
	}
	deviceID := parts[0]
	command := parts[1]

	device := s.store.GetDevice(deviceID)
	if device == nil {
		s.error(w, "device not found", 404)
		return
	}

	// Extract phone base URL from device URL (e.g. http://192.0.2.10:8080/video → http://192.0.2.10:8080)
	phoneURL := ""
	if strings.HasPrefix(device.URL, "http://") || strings.HasPrefix(device.URL, "https://") {
		parts := strings.Split(device.URL, "/")
		phoneURL = parts[0] + "//" + parts[2]
	}

	if phoneURL == "" {
		s.error(w, "not a HTTP device", 400)
		return
	}

	targetURL := phoneURL + "/api/" + command
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Use raw HTTP over TCP to avoid Go's http.Client compatibility issues with NanoHTTPD
	resp, err := rawHTTPRequest(r.Method, targetURL)
	if err != nil {
		s.error(w, "phone unreachable: "+err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.error(w, err.Error(), 502)
		return
	}
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func rawHTTPRequest(method, urlStr string) (*http.Response, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" { host += ":443" } else { host += ":80" }
	}
	conn, err := net.DialTimeout("tcp", host, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	path := u.Path
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	reqStr := method + " " + path + " HTTP/1.0\r\nHost: " + u.Host + "\r\nConnection: close\r\nAccept: */*\r\n\r\n"
	if _, err := conn.Write([]byte(reqStr)); err != nil {
		return nil, err
	}

	// Read raw response
	raw, err := io.ReadAll(conn)
	if err != nil {
		return nil, fmt.Errorf("read failed: %v", err)
	}
	// Parse manually
	parts := bytes.SplitN(raw, []byte("\r\n\r\n"), 2)
	headerLines := bytes.Split(parts[0], []byte("\r\n"))
	statusLine := string(headerLines[0])
	statusParts := strings.SplitN(statusLine, " ", 3)
	statusCode := 200
	if len(statusParts) >= 2 {
		statusCode, _ = strconv.Atoi(statusParts[1])
	}
	body := []byte{}
	if len(parts) > 1 {
		body = parts[1]
	}
	return &http.Response{
		Status:     statusLine,
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}, nil
}

func listV4L2Controls(devPath string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "v4l2-ctl", "-d", devPath, "--list-ctrls")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("v4l2-ctl failed: %v", err)
	}

	var controls []map[string]interface{}
	lineRe := regexp.MustCompile(`^\s+(\S+)\s+0x[0-9a-f]+\s+\((\w+)\)\s*:\s*(.+)$`)

	for _, line := range strings.Split(string(out), "\n") {
		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ctrl := map[string]interface{}{
			"name": m[1],
			"type": m[2],
		}
		props := m[3]

		pairRe := regexp.MustCompile(`(\w+)=(-?[\w.]+)`)
		for _, pair := range pairRe.FindAllStringSubmatch(props, -1) {
			switch pair[1] {
			case "min", "max", "step", "default":
				if v, err := strconv.Atoi(pair[2]); err == nil {
					ctrl[pair[1]] = v
				}
			case "value":
				if v, err := strconv.Atoi(pair[2]); err == nil {
					ctrl[pair[1]] = v
				} else {
					ctrl[pair[1]] = pair[2]
				}
			case "flags":
				ctrl["flags"] = pair[2]
			}
		}

		controls = append(controls, ctrl)
	}

	if len(controls) == 0 {
		return nil, fmt.Errorf("no controls found for %s", devPath)
	}
	return controls, nil
}

func setV4L2Control(devPath, control, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "v4l2-ctl", "-d", devPath,
		"--set-ctrl", fmt.Sprintf("%s=%s", control, value))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("v4l2-ctl set failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.error(w, "not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="utf-8"><title>视频流管理平台</title>
<style>body{font-family:sans-serif;margin:40px;line-height:1.6}</style>
</head>
<body>
<h1>视频流管理平台</h1>
<p>状态: <span style="color:green">运行中</span></p>
<a href="/ui/index.html">进入管理界面</a>
</body></html>`)
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.json(w, s.store.ListDevices())
	case "POST":
		var d Device
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			s.error(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		s.store.AddDevice(&d)
		s.json(w, d)
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleDeviceByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	id = strings.TrimRight(id, "/")

	switch r.Method {
	case "GET":
		d := s.store.GetDevice(id)
		if d == nil {
			s.error(w, "device not found", 404)
			return
		}
		// Check online status
		d.Status = checkDeviceOnline(d.URL)
		s.store.UpdateDeviceStatus(id, d.Status)
		s.json(w, d)
	case "PUT":
		var req struct {
			Name      string `json:"name,omitempty"`
			GroupName string `json:"group_name,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.error(w, "invalid JSON", 400)
			return
		}
		if req.GroupName != "" {
			s.store.UpdateDeviceGroup(id, req.GroupName)
		}
		s.json(w, map[string]string{"status": "updated"})
	case "DELETE":
		s.store.DeleteDevice(id)
		s.json(w, map[string]string{"status": "deleted"})
	default:
		s.error(w, "method not allowed", 405)
	}
}

// shortID returns the first 8 characters of a device ID, or the full
// ID if shorter than 8.  Used for logging prefixes.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func checkDeviceOnline(url string) string {
	// Try connecting to the RTSP port
	parts := strings.SplitN(url, "://", 2)
	if len(parts) < 2 {
		return "offline"
	}
	hostPort := strings.Split(parts[1], "/")[0]
	conn, err := net.DialTimeout("tcp", hostPort, 3*time.Second)
	if err != nil {
		return "offline"
	}
	conn.Close()
	return "online"
}

func (s *Server) handleRecordings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		deviceID := r.URL.Query().Get("device_id")
		s.json(w, s.store.ListRecordings(deviceID))
	case "POST":
		var req struct {
			DeviceID string `json:"device_id"`
			Duration int    `json:"duration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.error(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		rec, err := s.ffmpeg.StartRecording(req.DeviceID, req.Duration)
		if err != nil {
			s.error(w, err.Error(), 500)
			return
		}
		s.json(w, rec)
	case "DELETE":
		id := r.URL.Query().Get("id")
		if id == "" {
			s.error(w, "missing id", 400)
			return
		}
		s.store.DeleteRecording(id)
		s.json(w, map[string]string{"status": "deleted"})
	default:
		s.error(w, "method not allowed", 405)
	}
}

// requestHostname returns the host the client used to reach this API, without port.
func requestHostname(r *http.Request) string {
	host := r.Host
	if i := strings.LastIndexByte(host, ':'); i > 0 && !strings.HasPrefix(host, "[") {
		host = host[:i]
	}
	return host
}

func (s *Server) handleProxyStart(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/proxy/start/")
	deviceID = strings.TrimRight(deviceID, "/")

	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}

	// Parse optional query params for resolution override
	cfg := ProxyConfig{}
	if w := r.URL.Query().Get("width"); w != "" {
		if v, err := strconv.Atoi(w); err == nil {
			cfg.Width = v
		}
	}
	if h := r.URL.Query().Get("height"); h != "" {
		if v, err := strconv.Atoi(h); err == nil {
			cfg.Height = v
		}
	}
	if br := r.URL.Query().Get("bitrate"); br != "" {
		cfg.Bitrate = br
	}
	if vf := r.URL.Query().Get("vf"); vf != "" {
		cfg.VideoFilter = vf
	}

	if err := s.ffmpeg.StartLiveProxyWithConfig(deviceID, cfg); err != nil {
		s.error(w, err.Error(), 500)
		return
	}
	s.json(w, map[string]interface{}{
		"status":   "started",
		"deviceId": deviceID,
		"hls_url":  fmt.Sprintf("http://%s:8888/live/%s/index.m3u8", requestHostname(r), deviceID),
	})
}

func (s *Server) handleProxyStop(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/proxy/stop/")
	deviceID = strings.TrimRight(deviceID, "/")

	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}

	if err := s.ffmpeg.StopLiveProxy(deviceID); err != nil {
		s.error(w, err.Error(), 500)
		return
	}
	s.json(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.json(w, s.store.ListSchedules())
	case "POST":
		var sc Schedule
		if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
			s.error(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		s.store.AddSchedule(&sc)
		s.json(w, sc)
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.json(w, s.store.ListSnapshotConfigs())
	case "POST":
		var sn SnapshotConfig
		if err := json.NewDecoder(r.Body).Decode(&sn); err != nil {
			s.error(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		s.store.AddSnapshotConfig(&sn)
		s.json(w, sn)
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleSnapshotByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/snapshots/")
	id = strings.TrimRight(id, "/")
	// Check if it's /api/snapshots/list/ prefix
	if strings.HasPrefix(id, "list/") {
		s.handleSnapshotList(w, r)
		return
	}
	switch r.Method {
	case "GET":
		sn := s.store.GetSnapshotConfig(id)
		if sn == nil {
			s.error(w, "not found", 404)
			return
		}
		s.json(w, sn)
	case "PUT":
		var sn SnapshotConfig
		if err := json.NewDecoder(r.Body).Decode(&sn); err != nil {
			s.error(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		sn.ID = id
		s.store.UpdateSnapshotConfig(&sn)
		s.json(w, s.store.GetSnapshotConfig(id))
	case "DELETE":
		s.store.DeleteSnapshotConfig(id)
		s.json(w, map[string]string{"status": "deleted"})
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleSnapshotList(w http.ResponseWriter, r *http.Request) {
	// /api/snapshots/list/{configID}
	id := strings.TrimPrefix(r.URL.Path, "/api/snapshots/list/")
	id = strings.TrimRight(id, "/")
	snapDir := filepath.Join(s.dataDir, "snapshots", id)
	os.MkdirAll(snapDir, 0755)

	entries, err := os.ReadDir(snapDir)
	if err != nil {
		s.json(w, []string{})
		return
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || e.Name() == "latest.jpg" {
			continue
		}
		fi, _ := e.Info()
		if fi != nil {
			files = append(files, fi.Name())
		}
	}
	s.json(w, files)
}

func (s *Server) handleScheduleByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/schedules/")
	id = strings.TrimRight(id, "/")

	switch r.Method {
	case "GET":
		sc := s.store.GetSchedule(id)
		if sc == nil {
			s.error(w, "not found", 404)
			return
		}
		s.json(w, sc)
	case "PUT":
		var sc Schedule
		if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
			s.error(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		sc.ID = id
		s.store.UpdateSchedule(&sc)
		s.json(w, s.store.GetSchedule(id))
	case "DELETE":
		s.store.DeleteSchedule(id)
		s.json(w, map[string]string{"status": "deleted"})
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	devices, _, recordings := s.store.Stats()
	onlineCount := 0
	devList := s.store.ListDevices()
	for range devList {
		onlineCount++
	}

	s.json(w, map[string]interface{}{
		"status":           "running",
		"version":          "0.8.4",
		"devices":          devices,
		"online_devices":   onlineCount,
		"recordings":       recordings,
		"mediamtx_hls_url": "http://" + requestHostname(r) + ":8888/",
		"disk":             s.store.GetDiskStats(),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	devices, _, recordings := s.store.Stats()
	onlineCount := 0
	devList := s.store.ListDevices()
	for _, d := range devList {
		if checkDeviceOnline(d.URL) == "online" {
			onlineCount++
		}
	}

	s.json(w, map[string]interface{}{
		"status":           "ok",
		"version":          "0.8.4",
		"uptime":           time.Since(startTime).String(),
		"devices_total":    devices,
		"devices_online":   onlineCount,
		"recordings":       recordings,
		"ffmpeg_available": checkFFmpeg(),
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := s.ffmpeg.CollectMetrics()
	devices, _, recordings := s.store.Stats()
	metrics["devices"] = devices
	metrics["recordings"] = recordings
	metrics["tokens"] = len(s.ffmpeg.sem)
	s.json(w, metrics)
}

func checkFFmpeg() bool {
	cmd := exec.Command("ffmpeg", "-version")
	return cmd.Run() == nil
}

var startTime = time.Now()

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.error(w, "method not allowed", 405)
		return
	}
	backup := s.store.ExportAll()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=video-stream-manager-backup.json")
	w.Write([]byte(backup))
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" && r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}
	if len(body) == 0 {
		s.error(w, "empty body", 400)
		return
	}
	if err := s.store.ImportAll(string(body)); err != nil {
		s.error(w, "import failed: "+err.Error(), 500)
		return
	}
	s.json(w, map[string]string{"status": "imported"})
}

func (s *Server) startScheduler() {
	// Track currently recording schedules to avoid duplicates
	active := make(map[string]bool) // scheduleID -> true
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		nowMin := now.Hour()*60 + now.Minute()
		nowDay := int(now.Weekday()) // 0=Sunday
		dayStr := fmt.Sprintf("%d", nowDay)

		for _, sc := range s.store.ListSchedules() {
			if !sc.Enabled {
				continue
			}

			// Check day match
			if sc.Days != "" {
				days := strings.Split(sc.Days, ",")
				match := false
				for _, d := range days {
					if strings.TrimSpace(d) == dayStr {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}

			// Parse times
			startParts := strings.Split(sc.TimeStart, ":")
			endParts := strings.Split(sc.TimeEnd, ":")
			if len(startParts) < 2 || len(endParts) < 2 {
				continue
			}
			sh, _ := strconv.Atoi(startParts[0])
			sm, _ := strconv.Atoi(startParts[1])
			eh, _ := strconv.Atoi(endParts[0])
			em, _ := strconv.Atoi(endParts[1])
			startMin := sh*60 + sm
			endMin := eh*60 + em

			inRange := false
			if endMin > startMin {
				inRange = nowMin >= startMin && nowMin < endMin
			} else {
				// Crosses midnight
				inRange = nowMin >= startMin || nowMin < endMin
			}

			if inRange && !active[sc.ID] {
				// Start recording
				dur := sc.DurationMin
				if dur <= 0 {
					dur = 10 // default 10 min segments
				}
				log.Printf("[scheduler] starting recording for schedule %s (%s)", sc.ID[:8], sc.Name)
				rec, err := s.ffmpeg.StartRecording(sc.DeviceID, dur*60)
				if err != nil {
					log.Printf("[scheduler] recording failed for %s: %v", sc.ID[:8], err)
					continue
				}
				active[sc.ID] = true
				sc.LastTriggered = now.Format(time.RFC3339)
				s.store.UpdateSchedule(sc)
				log.Printf("[scheduler] recording %s started, file=%s", rec.ID[:8], rec.FilePath)
			} else if !inRange && active[sc.ID] {
				// Out of range, mark as done
				active[sc.ID] = false
				log.Printf("[scheduler] schedule %s ended", sc.ID[:8])
			}
		}
	}
}

func (s *Server) startSnapshotter() {
	snapDir := filepath.Join(s.dataDir, "snapshots")
	os.MkdirAll(snapDir, 0755)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for _, sn := range s.store.ListSnapshotConfigs() {
			if !sn.Enabled || sn.Interval < 5 {
				continue
			}
			device := s.store.GetDevice(sn.DeviceID)
			if device == nil {
				continue
			}
			// Check if it's time for a new snapshot
			lastSnap := filepath.Join(snapDir, sn.ID, "latest.jpg")
			shouldSnap := false
			if fi, err := os.Stat(lastSnap); err != nil {
				shouldSnap = true
			} else {
				shouldSnap = time.Since(fi.ModTime()) > time.Duration(sn.Interval)*time.Second
			}
			if !shouldSnap {
				continue
			}
			// Capture
			os.MkdirAll(filepath.Join(snapDir, sn.ID), 0755)
			ts := time.Now().Format("20060102-150405")
			fp := filepath.Join(snapDir, sn.ID, ts+".jpg")
			inputURL := buildInputURL(device)
			cmd := exec.Command("ffmpeg",
				"-rtsp_transport", "tcp",
				"-i", inputURL,
				"-vframes", "1",
				"-q:v", "3",
				"-y", fp,
			)
			if err := cmd.Run(); err != nil {
				log.Printf("[snap-%s] capture failed: %v", sn.ID[:8], err)
				continue
			}
			// Copy as latest
			os.Remove(lastSnap)
			os.Symlink(ts+".jpg", lastSnap)
			log.Printf("[snap-%s] captured %s", sn.ID[:8], ts)

			// Cleanup old snapshots
			if sn.Retention > 0 {
				cutoff := time.Now().AddDate(0, 0, -sn.Retention)
				entries, _ := os.ReadDir(filepath.Join(snapDir, sn.ID))
				for _, e := range entries {
					if e.IsDir() || e.Name() == "latest.jpg" {
						continue
					}
					fi, _ := e.Info()
					if fi != nil && fi.ModTime().Before(cutoff) {
						os.Remove(filepath.Join(snapDir, sn.ID, e.Name()))
					}
				}
			}
		}
	}
}

// ============ Health Check & Metrics ============

func (m *FFmpegManager) healthCheckLoop() {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.runHealthCheck()
		case <-m.stopCh:
			return
		}
	}
}

func (m *FFmpegManager) runHealthCheck() {
	m.mu.Lock()
	for id, p := range m.proxies {
		if !p.Running || p.Cmd == nil || p.Cmd.Process == nil || p.Cmd.ProcessState != nil {
			m.healthFails[id] = 0
			continue
		}
		if !m.streamAlive(id) {
			m.healthFails[id]++
			if m.healthFails[id] >= 5 {
				log.Printf("[health] proxy %s no stream for %d checks — killing stuck ffmpeg PID %d",
					shortID(id), m.healthFails[id], p.Cmd.Process.Pid)
				// Force-kill: process is stuck in CLOSE-WAIT and won't exit on its own.
				// The ffmpeg exit handler will automatically restart it.
				p.Cmd.Process.Kill()
				m.healthFails[id] = 0
			}
		} else {
			m.healthFails[id] = 0
		}
	}
	m.mu.Unlock()
}

func (m *FFmpegManager) streamAlive(deviceID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:8888/live/%s/index.m3u8", deviceID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Host = "127.0.0.1:8888"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// MediaMTX returns 200 with playlist when stream is available,
	// or error JSON (possibly 404/200) when not available.
	// Check response body for error marker.
	if resp.StatusCode != 200 {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return false
	}
	return !strings.Contains(string(body), `"error"`)
}

func (m *FFmpegManager) CollectMetrics() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	activeProxies := 0
	for _, p := range m.proxies {
		if p.Running {
			activeProxies++
		}
	}
	return map[string]interface{}{
		"active_proxies": activeProxies,
		"max_concurrent": maxConcurrentFFmpeg,
		"retries":        len(m.retries),
	}
}

func (m *FFmpegManager) Stop() {
	close(m.stopCh)
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, p := range m.proxies {
		if p.cancel != nil {
			p.cancel()
		}
		delete(m.proxies, id)
	}
}

// ============ PTZ Control ============

func (s *Server) handlePTZ(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	// /api/ptz/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/ptz/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		s.error(w, "invalid path", 400)
		return
	}
	deviceID := parts[0]
	action := parts[1]

	device := s.store.GetDevice(deviceID)
	if device == nil {
		s.error(w, "device not found", 404)
		return
	}

	// Parse host:port from device URL
	baseURL := strings.TrimSuffix(device.URL, "/ch1/main/av_stream")
	baseURL = strings.TrimSuffix(baseURL, "/ch1/sub/av_stream")
	baseURL = strings.TrimSuffix(baseURL, "/stream1")
	baseURL = strings.TrimSuffix(baseURL, "/Streaming/Channels/101")
	baseURL = strings.TrimSuffix(baseURL, ":554")
	if strings.HasPrefix(baseURL, "rtsp://") {
		baseURL = "http://" + strings.TrimPrefix(baseURL, "rtsp://")
	}
	hostPort := strings.TrimPrefix(baseURL, "http://")
	if !strings.Contains(hostPort, ":") {
		hostPort += ":80"
	}

	// Parse speed from query
	speedParam := r.URL.Query().Get("speed")
	speedVal := 0.3
	if speedParam != "" {
		if s, err := strconv.ParseFloat(speedParam, 64); err == nil && s > 0 && s <= 10 {
			speedVal = s / 10.0
		}
	}

	// Handle preset
	if strings.HasPrefix(action, "preset/") {
		presetNum := strings.TrimPrefix(action, "preset/")
		presetURL := fmt.Sprintf("http://%s/ISAPI/PTZCtrl/channels/1/presets/%s/goto", hostPort, presetNum)
		client := &http.Client{Timeout: 3 * time.Second}
		req, _ := http.NewRequest("PUT", presetURL, strings.NewReader("<PTZData><pan>0</pan><tilt>0</tilt><zoom>0</zoom></PTZData>"))
		req.Header.Set("Content-Type", "application/xml")
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			s.json(w, map[string]string{"status": "ok"})
		} else {
			s.error(w, "preset not supported", 404)
		}
		return
	}

	// Map action to Hikvision PTZ axis values
	var axis string
	speedStr := fmt.Sprintf("%.1f", speedVal)
	switch action {
	case "up":
		axis = "tilt"
	case "down":
		axis = "tilt"
		speedStr = "-" + speedStr
	case "left":
		axis = "pan"
		speedStr = "-" + speedStr
	case "right":
		axis = "pan"
	case "zoom_in":
		axis = "zoom"
	case "zoom_out":
		axis = "zoom"
		speedStr = "-" + speedStr
	case "stop":
		// Send stop to all axes
		xmlStop := `<PTZData><pan>0</pan><tilt>0</tilt><zoom>0</zoom></PTZData>`
		stopURL := fmt.Sprintf("http://%s/ISAPI/PTZCtrl/channels/1/continuous", hostPort)
		http.Post(stopURL, "application/xml", strings.NewReader(xmlStop))
		s.json(w, map[string]string{"status": "ok"})
		return
	default:
		s.error(w, "unknown action", 400)
		return
	}

	xmlBody := fmt.Sprintf(`<PTZData><%s>%s</%s></PTZData>`, axis, speedStr, axis)
	ptzURL := fmt.Sprintf("http://%s/ISAPI/PTZCtrl/channels/1/continuous", hostPort)

	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest("PUT", ptzURL, strings.NewReader(xmlBody))
	if err != nil {
		s.error(w, "request failed: "+err.Error(), 500)
		return
	}
	req.Header.Set("Content-Type", "application/xml")

	resp, err := client.Do(req)
	if err != nil {
		// Fallback: try older Hikvision CGI format
		fallbackURL := fmt.Sprintf("http://%s/cgi-bin/ptz.cgi?action=start&channel=1&code=%s&arg1=0&arg2=5",
			hostPort, mapActionToCGI(action))
		if resp2, err2 := http.Get(fallbackURL); err2 != nil {
			s.error(w, "PTZ not supported on this camera", 404)
			return
		} else {
			resp2.Body.Close()
		}
		s.json(w, map[string]string{"status": "ok", "method": "cgi"})
		return
	}
	defer resp.Body.Close()

	// Auto-stop after 500ms
	go func() {
		time.Sleep(500 * time.Millisecond)
		stopXML := `<PTZData><pan>0</pan><tilt>0</tilt><zoom>0</zoom></PTZData>`
		http.Post(ptzURL, "application/xml", strings.NewReader(stopXML))
	}()

	s.json(w, map[string]string{"status": "ok", "method": "isapi"})
}

func mapActionToCGI(action string) string {
	m := map[string]string{
		"up":       "Up",
		"down":     "Down",
		"left":     "Left",
		"right":    "Right",
		"zoom_in":  "ZoomIn",
		"zoom_out": "ZoomOut",
	}
	if v, ok := m[action]; ok {
		return v
	}
	return "Up"
}

func (s *Server) handleProxyConfig(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/proxy/config/")
	deviceID = strings.TrimRight(deviceID, "/")

	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}

	var cfg ProxyConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		s.error(w, "invalid JSON", 400)
		return
	}

	s.ffmpeg.StopLiveProxy(deviceID)
	s.ffmpeg.mu.Lock()
	s.ffmpeg.proxyCfgs[deviceID] = cfg
	s.ffmpeg.mu.Unlock()
	if err := s.ffmpeg.StartLiveProxyWithConfig(deviceID, cfg); err != nil {
		s.error(w, err.Error(), 500)
		return
	}
	s.json(w, map[string]interface{}{
		"status":   "updated",
		"deviceId": deviceID,
		"config":   cfg,
	})
}

// ============ Motion Detection ============

func (s *Server) handleMotionConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.json(w, s.store.ListMotionConfigs())
		return
	}
	s.error(w, "method not allowed", 405)
}

func (s *Server) handleMotionConfigByDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/motion/config/")
	deviceID = strings.TrimRight(deviceID, "/")

	switch r.Method {
	case "GET":
		cfg := s.store.GetMotionConfig(deviceID)
		if cfg == nil {
			cfg = &MotionConfig{DeviceID: deviceID, Enabled: false, Sensitivity: 0.3, CooldownSec: 30}
		}
		s.json(w, cfg)
	case "POST":
		var cfg MotionConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			s.error(w, "invalid JSON", 400)
			return
		}
		cfg.DeviceID = deviceID
		s.store.SetMotionConfig(&cfg)
		s.json(w, cfg)
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleMotionEvents(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/motion/events/")
	deviceID = strings.TrimRight(deviceID, "/")

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	if deviceID == "" || deviceID == "all" {
		since := time.Now().Add(-24 * time.Hour)
		s.json(w, s.store.ListMotionEventsAll(since))
		return
	}
	s.json(w, s.store.ListMotionEvents(deviceID, limit))
}

// motionDetect runs a quick scene detect on a device's stream
func (s *Server) motionDetect(deviceID string) (float64, error) {
	device := s.store.GetDevice(deviceID)
	if device == nil {
		return 0, fmt.Errorf("device not found")
	}
	inputURL := buildInputURL(device)

	// Run ffmpeg for 2s with scene detect filter
	cmd := exec.Command("ffprobe",
		"-rtsp_transport", "tcp",
		"-i", inputURL,
		"-f", "null",
		"-",
	)
	// ffprobe doesn't support scene detect directly, use ffmpeg with select filter
	cmd = exec.Command("ffmpeg",
		"-rtsp_transport", "tcp",
		"-i", inputURL,
		"-vf", fmt.Sprintf("select='gt(scene,0.3)',showinfo"),
		"-f", "null",
		"-t", "2",
		"-an",
		"-y", "/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ffmpeg: %v", string(out))
	}
	// Parse showinfo output for scene score
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "pts_time:") && strings.Contains(line, "score:") {
			parts := strings.Split(line, "score:")
			if len(parts) == 2 {
				scoreStr := strings.TrimSpace(parts[1])
				if score, err := strconv.ParseFloat(scoreStr, 64); err == nil {
					return score, nil
				}
			}
		}
	}
	return 0, nil
}

// motionDetectLoop runs periodically to check for motion on enabled devices
func (s *Server) startMotionDetector() {
	snapDir := filepath.Join(s.dataDir, "snapshots")
	os.MkdirAll(snapDir, 0755)
	lastDetect := make(map[string]time.Time)
	activeMotionRecs := make(map[string]*motionRec) // deviceID → active recording
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		cfgs := s.store.ListMotionConfigs()
		for _, cfg := range cfgs {
			if !cfg.Enabled {
				// If motion detection disabled but recording still active, stop it
				if rec, ok := activeMotionRecs[cfg.DeviceID]; ok {
					rec.cancel()
					delete(activeMotionRecs, cfg.DeviceID)
					log.Printf("[motion] stopped recording for disabled config %s", cfg.DeviceID[:8])
				}
				continue
			}

			device := s.store.GetDevice(cfg.DeviceID)
			if device == nil {
				continue
			}

			// Check cooldown — skip motion check if still in cooldown (reduces ffmpeg load)
			shouldDetect := true
			if last, ok := lastDetect[cfg.DeviceID]; ok && time.Since(last) < time.Duration(cfg.CooldownSec)*time.Second {
				shouldDetect = false
			}

			if shouldDetect {
				ts := time.Now().Format("20060102-150405")
				fp := filepath.Join(snapDir, "motion_"+cfg.DeviceID, ts+".jpg")
				os.MkdirAll(filepath.Dir(fp), 0755)

				inputURL := buildInputURL(device)
				detectArgs := []string{"-i", inputURL}
				if strings.HasPrefix(inputURL, "rtsp://") || strings.HasPrefix(inputURL, "rtsps://") {
					detectArgs = append([]string{"-rtsp_transport", "tcp"}, detectArgs...)
				}
				detectArgs = append(detectArgs,
					"-vf", fmt.Sprintf("select='gt(scene,%.1f)',showinfo", cfg.Sensitivity),
					"-vframes", "1",
					"-q:v", "3",
					"-f", "null",
					"-t", "2",
					"-y", fp,
				)
				cmd := exec.Command("ffmpeg", detectArgs...)
				out, _ := cmd.CombinedOutput()

				if strings.Contains(string(out), "pts_time:") {
					lastDetect[cfg.DeviceID] = time.Now()

					// Capture clean snapshot
					snapFP := filepath.Join(snapDir, "motion_"+cfg.DeviceID, ts+".jpg")
					snapArgs := []string{"-i", inputURL}
					if strings.HasPrefix(inputURL, "rtsp://") || strings.HasPrefix(inputURL, "rtsps://") {
						snapArgs = append([]string{"-rtsp_transport", "tcp"}, snapArgs...)
					}
					snapArgs = append(snapArgs, "-vframes", "1", "-q:v", "3", "-y", snapFP)
					snapCmd := exec.Command("ffmpeg", snapArgs...)
					snapCmd.Run()

					event := &MotionEvent{
						DeviceID:     cfg.DeviceID,
						DetectedAt:   time.Now().Format(time.RFC3339),
						SnapshotPath: snapFP,
					}
					s.store.AddMotionEvent(event)
					log.Printf("[motion] detected on %s, snapshot=%s", cfg.DeviceID[:8], snapFP)

					// Broadcast motion event via SSE
					s.broadcastSSE("motion", map[string]interface{}{
						"device_id": cfg.DeviceID,
						"device_name": device.Name,
						"detected_at": event.DetectedAt,
					})

					// Face detection on snapshot (async)
					go s.processFacesForMotion(snapFP, event.ID, cfg.DeviceID)

					// Send snapshot to VPS for AI analysis
					if s.vpsPluginURL != "" && s.vpsAPIKey != "" {
						go s.sendForAIAnalysis(cfg.DeviceID, device.Name, snapFP)
					}

					// *** START RECORDING on motion ***
					if _, already := activeMotionRecs[cfg.DeviceID]; !already {
						rec, err := s.ffmpeg.StartMotionRecording(cfg.DeviceID)
						if err != nil {
							log.Printf("[motion] failed to start recording for %s: %v", cfg.DeviceID[:8], err)
						} else {
							activeMotionRecs[cfg.DeviceID] = rec
							log.Printf("[motion] recording started for %s → %s", cfg.DeviceID[:8], rec.filePath)
						}
					}
				}
			}

			// *** STOP RECORDING if cooldown expired since last motion ***
			if rec, ok := activeMotionRecs[cfg.DeviceID]; ok {
				if last, ok := lastDetect[cfg.DeviceID]; ok && time.Since(last) >= time.Duration(cfg.CooldownSec)*time.Second {
					rec.cancel()
					delete(activeMotionRecs, cfg.DeviceID)
					log.Printf("[motion] stopped recording for %s (no motion for %ds)", cfg.DeviceID[:8], cfg.CooldownSec)
				}
			}
		}
	}
}

// ============ Phone Camera Streaming ============

type PhoneStream struct {
	mu      sync.Mutex
	frame   []byte
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	running bool
}

var phoneStreams = make(map[string]*PhoneStream)
var phoneMu sync.Mutex

func (s *Server) handlePhoneFrame(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/phone/frame/")
	deviceID = strings.TrimRight(deviceID, "/")

	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) < 100 {
		s.error(w, "invalid frame", 400)
		return
	}

	phoneMu.Lock()
	ps, ok := phoneStreams[deviceID]
	if !ok {
		ps = &PhoneStream{}
		phoneStreams[deviceID] = ps
		// Start FFmpeg process to push to MediaMTX
		go func() {
			rtmpDest := fmt.Sprintf("rtmp://127.0.0.1:1935/live/%s", deviceID)
			cmd := exec.Command("ffmpeg",
				"-f", "image2pipe",
				"-framerate", "10",
				"-i", "-",
				"-c:v", "libx264",
				"-preset", "ultrafast",
				"-tune", "zerolatency",
				"-b:v", "2000k",
				"-maxrate", "2000k",
				"-bufsize", "2000k",
				"-s", "1280x720",
				"-r", "15",
				"-pix_fmt", "yuv420p",
				"-g", "15",
				"-f", "flv",
				"-rtmp_live", "live",
				"-flvflags", "no_duration_filesize",
				rtmpDest,
			)
			stdin, _ := cmd.StdinPipe()
			ps.mu.Lock()
			ps.cmd = cmd
			ps.stdin = stdin
			ps.running = true
			ps.mu.Unlock()
			if err := cmd.Start(); err != nil {
				log.Printf("[phone-%s] ffmpeg start failed: %v", shortID(deviceID), err)
				ps.mu.Lock()
				ps.running = false
				ps.mu.Unlock()
				return
			}
			// Write any buffered frames
			ps.mu.Lock()
			if ps.frame != nil {
				stdin.Write(ps.frame)
			}
			ps.mu.Unlock()
			cmd.Wait()
			ps.mu.Lock()
			ps.running = false
			ps.mu.Unlock()
		}()
	}
	// Write frame
	ps.mu.Lock()
	ps.frame = body
	if ps.running && ps.stdin != nil {
		ps.stdin.Write(body)
	}
	ps.mu.Unlock()
	phoneMu.Unlock()

	w.WriteHeader(200)
}

// ============ Device Discovery ============

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.error(w, "method not allowed", 405)
		return
	}
	// Return current discovery progress
	s.json(w, s.disco.GetProgress())
}

func (s *Server) handleDiscoverStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	localIP := getLocalIP()
	if err := s.disco.StartScan(localIP); err != nil {
		s.error(w, err.Error(), 400)
		return
	}
	s.json(w, map[string]string{"status": "scan_started"})
}

func (s *Server) handleDiscoverStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	s.disco.StopScan()
	s.json(w, map[string]string{"status": "scan_stopped"})
}

func (s *Server) handleDiscoverStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.error(w, "method not allowed", 405)
		return
	}
	s.json(w, s.disco.GetProgress())
}

func (s *Server) handleDiscoverImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	var req struct {
		IPs []string `json:"ips"` // which discovered device IPs to import
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.error(w, "invalid JSON", 400)
		return
	}

	prog := s.disco.GetProgress()
	imported := 0
	for _, ip := range req.IPs {
		dd, ok := prog.Results[ip]
		if !ok {
			continue
		}
		// Determine the URL to use
		url := ""
		protocol := "rtsp"
		if len(dd.RTSPURLs) > 0 {
			url = dd.RTSPURLs[0]
			protocol = dd.Protocol
		} else if dd.HTTPURL != "" {
			url = dd.HTTPURL
			protocol = "http-mjpeg"
		} else if dd.Protocol == "usb" {
			url = dd.HWAddr
			protocol = "usb"
		}
		if url == "" {
			continue
		}

		name := dd.BrandCN
		if name == "" {
			name = "Camera-" + dd.IP
		}
		device := &Device{
			ID:        generateID(),
			Name:      name,
			Protocol:  protocol,
			URL:       url,
			Status:    "offline",
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		s.store.AddDevice(device)
		imported++
		log.Printf("[discovery] imported device: %s (%s)", device.Name, url)

		// Auto-start proxy
		go s.ffmpeg.StartLiveProxy(device.ID)
	}
	s.json(w, map[string]interface{}{
		"status":    "ok",
		"imported":  imported,
		"requested": len(req.IPs),
	})
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ============ Auth Handlers ============

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.error(w, "invalid JSON", 400)
		return
	}
	if req.Password != adminPassword {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "wrong password"})
		return
	}
	sid := generateSessionID()
	sessions.mu.Lock()
	sessions.m[sid] = time.Now().Add(sessionDuration)
	sessions.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sid,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	s.json(w, map[string]string{"status": "ok"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	c, err := r.Cookie("session_id")
	if err == nil && c.Value != "" {
		sessions.mu.Lock()
		delete(sessions.m, c.Value)
		sessions.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
		s.json(w, map[string]string{"status": "logged_out"})
}

// ============ License Handlers ============

func (s *Server) handleLicenseGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	var req struct {
		IsPro bool   `json:"is_pro"`
		Note  string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.error(w, "invalid JSON", 400)
		return
	}
	key := s.store.GenerateLicense(req.IsPro, req.Note)
	s.json(w, map[string]interface{}{"key": key, "status": "created"})
}

func (s *Server) handleLicenseCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.error(w, "invalid JSON", 400)
		return
	}
	lic := s.store.CheckLicense(req.Key)
	if lic == nil {
		s.json(w, map[string]interface{}{"valid": false, "is_pro": false})
		return
	}
	s.json(w, map[string]interface{}{"valid": true, "is_pro": lic.IsPro, "device_id": lic.DeviceID})
}

func (s *Server) handleLicenseBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Key      string `json:"key"`
		DeviceID string `json:"device_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.error(w, "invalid JSON", 400)
		return
	}
	if s.store.BindLicense(req.Key, req.DeviceID) {
		s.json(w, map[string]string{"status": "bound"})
	} else {
		s.error(w, "license already bound or not found", 400)
	}
}

func (s *Server) handleLicenseList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.error(w, "method not allowed", 405)
		return
	}
	lics := s.store.ListLicenses()
	if lics == nil {
		lics = []*License{}
	}
	s.json(w, lics)
}

// ============ Face API Handlers ============

func (s *Server) handleFaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		faces := s.store.ListFaces()
		if faces == nil {
			faces = []*KnownFace{}
		}
		s.json(w, faces)
	case "POST":
		var req struct {
			Label    string `json:"label"`
			FaceHash string `json:"face_hash,omitempty"`
			DeviceID string `json:"device_id,omitempty"`
			ThumbPath string `json:"thumb_path,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.error(w, "invalid JSON", 400)
			return
		}
		f := &KnownFace{
			Label:     req.Label,
			FaceHash:  req.FaceHash,
			DeviceID:  req.DeviceID,
			ThumbPath: req.ThumbPath,
		}
		s.store.AddFace(f)
		s.json(w, f)
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleFaceByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/faces/")
	id = strings.TrimRight(id, "/")
	if id == "" {
		s.handleFaces(w, r)
		return
	}

	switch r.Method {
	case "GET":
		f := s.store.GetFace(id)
		if f == nil {
			s.error(w, "face not found", 404)
			return
		}
		s.json(w, f)
	case "PUT":
		var req struct {
			Label string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.error(w, "invalid JSON", 400)
			return
		}
		s.store.UpdateFaceLabel(id, req.Label)
		s.json(w, map[string]string{"status": "updated"})
	case "DELETE":
		s.store.DeleteFace(id)
		s.json(w, map[string]string{"status": "deleted"})
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleFaceEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.error(w, "method not allowed", 405)
		return
	}
	deviceID := r.URL.Query().Get("device_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	events := s.store.ListFaceEvents(deviceID, limit)
	if events == nil {
		events = []*FaceEvent{}
	}
	s.json(w, events)
}

func (s *Server) handleFaceEventsByMotion(w http.ResponseWriter, r *http.Request) {
	motionID := strings.TrimPrefix(r.URL.Path, "/api/face-events/")
	motionID = strings.TrimRight(motionID, "/")
	if motionID == "" {
		s.handleFaceEvents(w, r)
		return
	}
	events := s.store.ListFaceEventsByMotion(motionID)
	if events == nil {
		events = []*FaceEvent{}
	}
	s.json(w, events)
}

// ============ SSE (Server-Sent Events) ============

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleTalk receives PCM audio from browser WebSocket and pipes to FFmpeg → MediaMTX.
func (s *Server) handleTalk(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/talk/")
	deviceID = strings.TrimRight(deviceID, "/")
	if deviceID == "" {
		http.Error(w, "device_id required", 400)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[talk] upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Start FFmpeg: read PCM s16le 16kHz mono from stdin, encode AAC, push to MediaMTX
	// MediaMTX will make it available as rtmp://localhost:1935/talk/{deviceID}
	ffmpeg := exec.Command("ffmpeg",
		"-f", "s16le",
		"-ar", "16000",
		"-ac", "1",
		"-i", "pipe:0",
		"-c:a", "aac",
		"-b:a", "64k",
		"-f", "flv",
		fmt.Sprintf("rtmp://localhost:1935/talk/%s", deviceID),
	)

	stdin, err := ffmpeg.StdinPipe()
	if err != nil {
		log.Printf("[talk] stdin pipe error: %v", err)
		return
	}

	if err := ffmpeg.Start(); err != nil {
		log.Printf("[talk] ffmpeg start error: %v", err)
		return
	}

	log.Printf("[talk] started for device %s", deviceID[:8])

	// Read WebSocket binary messages and write to FFmpeg stdin
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		stdin.Write(msg)
	}

	// Cleanup
	stdin.Close()
	ffmpeg.Process.Kill()
	ffmpeg.Wait()
	log.Printf("[talk] stopped for device %s", deviceID[:8])
}

// handleSSE streams motion/face events to the browser via SSE.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan string, 16)
	s.sseMu.Lock()
	s.sseClients[ch] = true
	s.sseMu.Unlock()

	// Remove client on disconnect
	notify := r.Context().Done()
	go func() {
		<-notify
		s.sseMu.Lock()
		delete(s.sseClients, ch)
		s.sseMu.Unlock()
	}()

	// Send initial keepalive
	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	for msg := range ch {
		fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
	}
}

// broadcastSSE sends a JSON event to all connected SSE clients.
func (s *Server) broadcastSSE(eventType string, data interface{}) {
	payload, err := json.Marshal(map[string]interface{}{
		"type": eventType,
		"data": data,
	})
	if err != nil {
		return
	}
	msg := string(payload)
	s.sseMu.Lock()
	defer s.sseMu.Unlock()
	for ch := range s.sseClients {
		select {
		case ch <- msg:
		default:
			// Client too slow, drop
		}
	}
}

// ============ Groups API Handler ============

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.json(w, s.store.ListGroups())
	case "PUT":
		// Rename a group: {"old":"客厅","new":"起居室"}
		var req struct {
			Old string `json:"old"`
			New string `json:"new"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.error(w, "invalid JSON", 400)
			return
		}
		if req.Old == "" || req.New == "" {
			s.error(w, "old and new required", 400)
			return
		}
		// Update all devices with old group name
		s.store.RenameGroup(req.Old, req.New)
		s.json(w, map[string]string{"status": "renamed"})
	default:
		s.error(w, "method not allowed", 405)
	}
}

// ============ Storage API Handler ============

func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		rc := s.store.GetRetentionConfig()
		disk := s.store.GetDiskStats()
		s.json(w, map[string]interface{}{
			"retention": rc,
			"disk":      disk,
		})
	case "PUT":
		var rc RetentionConfig
		if err := json.NewDecoder(r.Body).Decode(&rc); err != nil {
			s.error(w, "invalid JSON", 400)
			return
		}
		s.store.UpdateRetentionConfig(&rc)
		s.json(w, map[string]string{"status": "updated"})
	case "POST":
		// Manual cleanup trigger
		rc := s.store.GetRetentionConfig()
		if !rc.Enabled {
			s.json(w, map[string]interface{}{"status": "skipped", "reason": "auto cleanup disabled"})
			return
		}
		deleted := s.store.DeleteOldRecordings(rc)
		s.json(w, map[string]interface{}{"status": "done", "deleted": deleted})
	default:
		s.error(w, "method not allowed", 405)
	}
}

// ============ Face Detection Integration ============

// processFacesForMotion is called after a motion event to detect and match faces in the snapshot.
func (s *Server) processFacesForMotion(snapshotPath, motionEventID, deviceID string) {
	if !s.faceDetector.Ready() {
		return
	}

	faces, err := s.faceDetector.DetectFaces(snapshotPath)
	if err != nil {
		log.Printf("[face] detection error on %s: %v", snapshotPath[:40], err)
		return
	}
	if len(faces) == 0 {
		return
	}

	// Load all known faces for matching
	knownFaces := s.store.ListFaces()
	log.Printf("[face] detected %d face(s) in motion event %s (known: %d)", len(faces), motionEventID[:8], len(knownFaces))

	for _, face := range faces {
		// Crop face from snapshot
		faceImg, err := CropFace(snapshotPath, face.Bounds)
		if err != nil {
			log.Printf("[face] crop error: %v", err)
			continue
		}

		// Save thumbnail
		thumbPath, err := SaveFaceThumbnail(faceImg, s.faceDir)
		if err != nil {
			log.Printf("[face] save thumb error: %v", err)
			continue
		}

		// Compute hash
		faceHash := AverageHash(faceImg)

		// Try to match against known faces
		boundsJSON, _ := json.Marshal(map[string]int{
			"x": face.Bounds.Min.X,
			"y": face.Bounds.Min.Y,
			"w": face.Bounds.Dx(),
			"h": face.Bounds.Dy(),
		})

		matchedFace, confidence := MatchFace(faceHash, knownFaces, 15)
		label := "unknown"
		faceID := ""
		if matchedFace != nil {
			label = matchedFace.Label
			faceID = matchedFace.ID
			// Update seen count
			s.store.UpdateFaceSeen(matchedFace.ID)
		} else {
			// New unknown face — add to known faces for future matching
			existing := s.store.FindFaceByHash(faceHash)
			if existing == nil {
				newFace := &KnownFace{
					Label:     "unknown",
					FaceHash:  faceHash,
					DeviceID:  deviceID,
					ThumbPath: thumbPath,
				}
				s.store.AddFace(newFace)
				faceID = newFace.ID
				knownFaces = append(knownFaces, newFace)
			} else {
				faceID = existing.ID
				label = existing.Label
				s.store.UpdateFaceSeen(existing.ID)
			}
		}

		// Record face event
		event := &FaceEvent{
			MotionEventID: motionEventID,
			DeviceID:      deviceID,
			FaceID:        faceID,
			Label:         label,
			Confidence:    confidence,
			Score:         face.Score,
			Bounds:        string(boundsJSON),
			ThumbPath:     thumbPath,
			DetectedAt:    time.Now().Format(time.RFC3339),
		}
		s.store.AddFaceEvent(event)
		log.Printf("[face] %s: %s (match=%s, conf=%.2f)", deviceID[:8], label, faceID[:8], confidence)
	}
}

// findCascadeFile locates the pigo facefinder cascade file.
// Checks several common paths.
func findCascadeFile(dataDir string) string {
	candidates := []string{
		"backend/facefinder",                          // run from project root
		filepath.Join(dataDir, "..", "backend", "facefinder"), // DATA_DIR=./data
		filepath.Join(dataDir, "facefinder"),           // copy in data dir
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	// Default to first candidate (will log warning in NewFaceDetector)
	return candidates[0]
}

// Update NewServer to start motion detector
func NewServer(store *Store, dataDir string) *Server {
	initAuth()
	jar, _ := cookiejar.New(nil)
	s := &Server{
		store:        store,
		ffmpeg:       NewFFmpegManager(store, dataDir),
		director:     NewDirector(NewFFmpegManager(store, dataDir), store, dataDir),
		disco:        NewDiscoveryManager(),
		mux:          http.NewServeMux(),
		dataDir:      dataDir,
		vpsPluginURL: os.Getenv("VPS_PLUGIN_URL"),
		vpsAPIKey:    os.Getenv("VPS_API_KEY"),
		faceDetector: NewFaceDetector(findCascadeFile(dataDir)),
		faceDir:      filepath.Join(dataDir, "faces"),
		sseClients:   make(map[chan string]bool),
		hlsClient:    &http.Client{Jar: jar},
	}
	s.registerRoutes()

	// Serve recorded video files
	videosDir := filepath.Join(dataDir, "videos")
	os.MkdirAll(videosDir, 0755)
	fs := http.FileServer(http.Dir(videosDir))
	s.mux.Handle("/videos/", http.StripPrefix("/videos/", fs))

	// Serve snapshot files
	snapsDir := filepath.Join(dataDir, "snapshots")
	os.MkdirAll(snapsDir, 0755)
	sfs := http.FileServer(http.Dir(snapsDir))
	s.mux.Handle("/snapshots/", http.StripPrefix("/snapshots/", sfs))

	// Serve face thumbnails
	os.MkdirAll(s.faceDir, 0755)
	ffs := http.FileServer(http.Dir(s.faceDir))
	s.mux.Handle("/faces/", http.StripPrefix("/faces/", ffs))

	// Start motion detector
	go s.startMotionDetector()

	// Start schedule checker
	go s.startScheduler()
	// Start snapshot capture
	go s.startSnapshotter()

	// Start subscription heartbeat
	s.startSubscriptionHeartbeat()

	// Start recording retention cleanup (every 6 hours)
	go func() {
		for {
			rc := s.store.GetRetentionConfig()
			if rc.Enabled {
				s.store.DeleteOldRecordings(rc)
			}
			time.Sleep(6 * time.Hour)
		}
	}()

	return s
}

// startSubscriptionHeartbeat — periodic check-in with VPS to verify subscription
func (s *Server) startSubscriptionHeartbeat() {
	if s.vpsPluginURL == "" || s.vpsAPIKey == "" {
		log.Println("[SUB] VPS not configured, skipping subscription heartbeat")
		return
	}
	go func() {
		s.doHeartbeat()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.doHeartbeat()
		}
	}()
}

func (s *Server) doHeartbeat() {
	url := strings.TrimRight(s.vpsPluginURL, "/") + "/api/client/subscription/check"
	body := map[string]string{
		"device_id": getHostname(),
		"version":   "0.8.4",
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		log.Printf("[SUB] heartbeat request failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.vpsAPIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[SUB] heartbeat connection failed: %v (VPS might be offline)", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Valid         bool   `json:"valid"`
			DaysRemaining int    `json:"days_remaining"`
			Reason        string `json:"reason"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[SUB] heartbeat decode failed: %v", err)
		return
	}

	if result.Success && result.Data.Valid {
		log.Printf("[SUB] heartbeat OK — %d days remaining", result.Data.DaysRemaining)
	} else {
		reason := result.Data.Reason
		if reason == "" {
			reason = "unknown"
		}
		log.Printf("[SUB] heartbeat FAILED — %s", reason)
	}
}

func getHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "gateway-unknown"
	}
	return name
}

// sendForAIAnalysis reads a snapshot JPEG and sends it to the VPS plugin for AI analysis
func (s *Server) sendForAIAnalysis(deviceID, cameraName, snapPath string) {
	// Read the snapshot file
	data, err := os.ReadFile(snapPath)
	if err != nil {
		log.Printf("[AI] failed to read snapshot %s: %v", snapPath, err)
		return
	}

	// Encode to base64
	b64 := base64.StdEncoding.EncodeToString(data)

	url := strings.TrimRight(s.vpsPluginURL, "/") + "/api/client/analyze"
	body := map[string]interface{}{
		"device_id":    deviceID,
		"camera_name":  cameraName,
		"image_base64": b64,
		"motion_score": 1.0,
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		log.Printf("[AI] request creation failed: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.vpsAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[AI] VPS connection failed: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Analysis   string `json:"analysis"`
			Confidence string `json:"confidence"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[AI] decode failed: %v", err)
		return
	}
	if result.Success {
		log.Printf("[AI] %s: %s (confidence: %s)", cameraName, result.Data.Analysis, result.Data.Confidence)
	} else {
		log.Printf("[AI] VPS returned error for %s", cameraName)
	}
}

func main() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/app/data"
	}
	os.MkdirAll(dataDir, 0755)

	videosDir := filepath.Join(dataDir, "videos")
	os.MkdirAll(videosDir, 0755)

	store := NewStore(videosDir, dataDir)
	server := NewServer(store, dataDir)

	addr := ":8899"
	log.Printf("=== 视频流管理平台 v0.8.4 ===")
	log.Printf("API 服务: http://0.0.0.0%s", addr)
	log.Printf("打开浏览器访问 http://localhost%s", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatal(err)
	}
}
