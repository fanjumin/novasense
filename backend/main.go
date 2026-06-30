package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	maxConcurrentFFmpeg = 4
	healthCheckInterval = 15 * time.Second
	backoffBase         = 3 * time.Second
	backoffMax          = 60 * time.Second
	sessionDuration     = 24 * time.Hour
)

// ============ Auth ============

var (
	adminPassword string
	sessions      = struct {
		mu sync.Mutex
		m  map[string]time.Time
	}{m: make(map[string]time.Time)}
)

func initAuth() {
	adminPassword = os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "admin"
		log.Println("[auth] ⚠️ ADMIN_PASSWORD 未设置，使用默认密码 'admin'，请立即设置环境变量")
	}
}

func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashPassword(pw string) string {
	h := sha256.Sum256([]byte(pw))
	return hex.EncodeToString(h[:])
}

func isAuthenticated(r *http.Request) bool {
	c, err := r.Cookie("session_id")
	if err != nil {
		return false
	}
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	expiry, ok := sessions.m[c.Value]
	if !ok || time.Now().After(expiry) {
		delete(sessions.m, c.Value)
		return false
	}
	// Slide expiry
	sessions.m[c.Value] = time.Now().Add(sessionDuration)
	return true
}

// PublicPaths — no auth required
var publicPaths = []string{
	"/api/login",
	"/api/health",
	"/api/license/check",
	"/ui/login.html",
	"/NetCamPro.apk",
}

func isPublicPath(path string) bool {
	for _, p := range publicPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// ============ Data Models (defined in store.go) ============
// ============ FFmpeg Process Manager ============

type FFmpegManager struct {
	mu      sync.Mutex
	proxies map[string]*FFmpegProc // live proxy relays
	store   *Store
	dataDir string

	sem        chan struct{} // concurrency semaphore
	retries    map[string]int
	stopCh     chan struct{}
	proxyCfgs  map[string]ProxyConfig // per-device proxy config
	wapaBin    string                  // path to wapa-pull helper binary
}

type FFmpegProc struct {
	ID      string
	Cmd     *exec.Cmd
	cancel  context.CancelFunc
	Running bool
	kind    string // "proxy" or "push"
	started time.Time
}

func NewFFmpegManager(store *Store, dataDir string) *FFmpegManager {
	m := &FFmpegManager{
		proxies: make(map[string]*FFmpegProc),
		store:   store,
		dataDir: dataDir,
		sem:        make(chan struct{}, maxConcurrentFFmpeg),
		retries:    make(map[string]int),
		stopCh:     make(chan struct{}),
		proxyCfgs:  make(map[string]ProxyConfig),
		wapaBin:    filepath.Join(filepath.Dir(os.Args[0]), "..", "bin", "wapa-pull"),
	}
	go m.healthCheckLoop()
	return m
}

// buildInputURL constructs RTSP URL with credentials
func buildInputURL(device *Device) string {
	if device.Username == "" {
		return device.URL
	}
	parts := strings.SplitN(device.URL, "://", 2)
	if len(parts) != 2 {
		return device.URL
	}
	return parts[0] + "://" + device.Username + ":" + device.Password + "@" + parts[1]
}

// ============ Push Task (to douyin/kuaishou/bilibili) ============

// ============ Live Proxy (RTSP → MediaMTX for web viewing) ============

type ProxyConfig struct {
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Bitrate string `json:"bitrate"`
}

var defaultProxyConfig = map[string]string{
	"3840x2160": "12000k",
	"2560x1440": "6000k",
	"1920x1080": "4000k",
	"1280x720":  "2000k",
	"854x480":   "1000k",
}

func (m *FFmpegManager) detectResolution(deviceID string) ProxyConfig {
	device := m.store.GetDevice(deviceID)
	if device == nil {
		return ProxyConfig{}
	}

	// WAPA cameras: known resolution 1280x720 (BL-720Q-L)
	if device.Protocol == "wapa" {
		return ProxyConfig{
			Width:   1280,
			Height:  720,
			Bitrate: "2000k",
		}
	}

	// Quick ffprobe to detect native resolution
	args := []string{"-v", "quiet", "-print_format", "json", "-show_streams", "-select_streams", "v:0"}
	if strings.HasPrefix(device.URL, "http://") || strings.HasPrefix(device.URL, "https://") {
		args = append([]string{"-f", "mjpeg"}, args...)
	}
	args = append(args, device.URL)

	cmd := exec.Command("ffprobe", args...)

	// Timeout after 5s to avoid hanging on unresponsive streams
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.Output()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		return ProxyConfig{}
	}
	if err != nil {
		return ProxyConfig{}
	}

	var result struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Streams) == 0 {
		return ProxyConfig{}
	}

	w := result.Streams[0].Width
	h := result.Streams[0].Height

	// Choose bitrate based on resolution
	var br string
	switch {
	case w >= 3840:
		br = "12000k"
	case w >= 2560:
		br = "6000k"
	case w >= 1920:
		br = "4000k"
	case w >= 1280:
		br = "2000k"
	default:
		br = "1000k"
	}

	return ProxyConfig{Width: w, Height: h, Bitrate: br}
}

func (m *FFmpegManager) buildProxyArgs(inputURL, rtmpDest string, cfg ProxyConfig) []string {
	w := cfg.Width
	if w == 0 {
		w = 1920
	}
	h := cfg.Height
	if h == 0 {
		h = 1080
	}
	br := cfg.Bitrate
	if br == "" {
		br = "4000k"
	}

	args := []string{
		"-fflags", "nobuffer",
		"-flags", "low_delay",
	}

	// WAPA cameras use pipe input from wapa-pull helper
	if strings.HasPrefix(inputURL, "wapa://") {
		args = append(args, "-f", "m4v", "-err_detect", "ignore_err", "-i", "pipe:0")
	} else {
		args = append(args, "-i", inputURL)
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-b:v", br,
		"-maxrate", br,
		"-bufsize", br,
		"-s", fmt.Sprintf("%dx%d", w, h),
		"-r", "15",
		"-pix_fmt", "yuv420p",
		"-g", "8",
		"-c:a", "aac",
		"-b:a", "64k",
		"-ar", "16000",
		"-ac", "1",
		"-f", "flv",
		"-rtmp_live", "live",
		"-flvflags", "no_duration_filesize",
		rtmpDest,
	)

	// Add RTSP-specific flags only for RTSP URLs
	if strings.HasPrefix(inputURL, "rtsp://") || strings.HasPrefix(inputURL, "rtsps://") {
		args = append([]string{"-rtsp_transport", "tcp"}, args...)
	}
	// For MJPEG/HTTP, don't force input format; ffmpeg auto-detects
	// For USB cameras (/dev/video*), add input format
	if strings.HasPrefix(inputURL, "/dev/video") {
		args = append([]string{"-f", "v4l2", "-input_format", "mjpeg", "-framerate", "15"}, args...)
	}
	// For HTTP MJPEG streams (IP Webcam style), force mjpeg demuxer
	if strings.HasPrefix(inputURL, "http://") || strings.HasPrefix(inputURL, "https://") {
		args = append([]string{"-f", "mjpeg"}, args...)
	}

	return args
}

func (m *FFmpegManager) StartLiveProxy(deviceID string) error {
	return m.StartLiveProxyWithConfig(deviceID, ProxyConfig{})
}

func (m *FFmpegManager) StartLiveProxyWithConfig(deviceID string, cfg ProxyConfig) error {
	device := m.store.GetDevice(deviceID)
	if device == nil {
		return fmt.Errorf("device %s not found", deviceID)
	}

	// Auto-detect resolution if not specified
	if cfg.Width == 0 || cfg.Height == 0 {
		detected := m.detectResolution(deviceID)
		if detected.Width > 0 {
			cfg.Width = detected.Width
			cfg.Height = detected.Height
			cfg.Bitrate = detected.Bitrate
			log.Printf("[proxy] detected resolution for %s: %dx%d %s", shortID(deviceID), cfg.Width, cfg.Height, cfg.Bitrate)
		}
	}

	m.mu.Lock()
	if p, ok := m.proxies[deviceID]; ok && p.Running {
		m.mu.Unlock()
		return nil // already running
	}
	m.mu.Unlock()

	// Acquire semaphore slot (blocks if at maxConcurrentFFmpeg)
	m.sem <- struct{}{}

	inputURL := buildInputURL(device)
	rtmpDest := fmt.Sprintf("rtmp://127.0.0.1:1935/live/%s", deviceID)

	// Save config for restart / toggle
	m.mu.Lock()
	if existing, ok := m.proxyCfgs[deviceID]; ok {
		cfg = existing
	}
	m.proxyCfgs[deviceID] = cfg
	m.mu.Unlock()

	args := m.buildProxyArgs(inputURL, rtmpDest, cfg)

	ctx, cancel := context.WithCancel(context.Background())

	var cmd *exec.Cmd
	// WAPA cameras: pipe wapa-pull output into ffmpeg stdin
	if device.Protocol == "wapa" {
		ipPort := strings.TrimPrefix(device.URL, "wapa://")
		if !strings.Contains(ipPort, ":") {
			ipPort += ":9001"
		}
		// Use a FIFO — m4v demuxer handles files better than pipe:0
		fifoPath := filepath.Join(os.TempDir(), fmt.Sprintf("wapa-%s.fifo", deviceID))
		os.Remove(fifoPath)
		syscall.Mkfifo(fifoPath, 0666)

		// Open FIFO for both read and write — this doesn't block (unlike O_WRONLY)
		fifoW, err := os.OpenFile(fifoPath, os.O_RDWR, 0)
		if err != nil {
			cancel()
			<-m.sem
			return fmt.Errorf("failed to open fifo: %v", err)
		}
		defer fifoW.Close()

		// Start wapa-pull in background, writing to FIFO
		pullCmd := exec.CommandContext(ctx, m.wapaBin, ipPort)
		pullCmd.Stdout = fifoW
		pullCmd.Stderr = nil
		if err := pullCmd.Start(); err != nil {
			cancel()
			<-m.sem
			return fmt.Errorf("failed to start wapa-pull: %v", err)
		}
		log.Printf("[proxy-%s] started wapa-pull for %s (pid %d)", shortID(deviceID), ipPort, pullCmd.Process.Pid)

		// ffmpeg reads from FIFO like a file
		fifoArgs := m.buildProxyArgs(fifoPath, rtmpDest, cfg)
		cmd = exec.CommandContext(ctx, "ffmpeg", fifoArgs...)
	} else {
		cmd = exec.CommandContext(ctx, "ffmpeg", args...)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		<-m.sem // release semaphore
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		<-m.sem
		return fmt.Errorf("failed to start ffmpeg: %v", err)
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[proxy-%s] %s", shortID(deviceID), scanner.Text())
		}
	}()

	// Reset retry count on successful start — and release semaphore on exit
	proc := &FFmpegProc{
		ID:      deviceID,
		Cmd:     cmd,
		cancel:  cancel,
		Running: true,
		kind:    "proxy",
		started: time.Now(),
	}

	m.mu.Lock()
	m.proxies[deviceID] = proc
	m.retries[deviceID] = 0
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		if p, ok := m.proxies[deviceID]; ok {
			p.Running = false
		}
		retries := m.retries[deviceID]
		m.mu.Unlock()

		// Release semaphore slot
		<-m.sem

		if err != nil {
			// Exponential backoff
			backoff := backoffBase * time.Duration(1<<uint(retries))
			if backoff > backoffMax {
				backoff = backoffMax
			}
			m.mu.Lock()
			m.retries[deviceID] = retries + 1
			// Only restart if still in map (wasn't explicitly stopped)
			_, shouldRestart := m.proxies[deviceID]
			m.mu.Unlock()

			if shouldRestart {
				log.Printf("[proxy-%s] exited: %v — retry %d, backoff %v",
					shortID(deviceID), err, retries+1, backoff)
				time.Sleep(backoff)
				m.StartLiveProxy(deviceID)
			}
		}
	}()

	return nil
}

func (m *FFmpegManager) StopLiveProxy(deviceID string) error {
	m.mu.Lock()
	p, ok := m.proxies[deviceID]
	delete(m.proxies, deviceID) // remove from map so auto-restart won't fire
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("proxy for device %s not running", deviceID)
	}

	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func (m *FFmpegManager) IsProxyRunning(deviceID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.proxies[deviceID]; ok && p.Running {
		return true
	}
	return false
}

// ============ Recording ============

func (m *FFmpegManager) StartRecording(deviceID string, duration int) (*Recording, error) {
	device := m.store.GetDevice(deviceID)
	if device == nil {
		return nil, fmt.Errorf("device %s not found", deviceID)
	}

	now := time.Now()
	dir := filepath.Join(m.dataDir, "videos", deviceID, now.Format("20060102"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create dir: %v", err)
	}

	filename := fmt.Sprintf("%s.mp4", now.Format("150405"))
	filePath := filepath.Join(dir, filename)

	inputURL := buildInputURL(device)

	durStr := fmt.Sprintf("%d", duration)
	if duration <= 0 {
		durStr = "600"
	}

	args := []string{
		"-rtsp_transport", "tcp",
		"-i", inputURL,
		"-t", durStr,
		"-c", "copy",
		"-y",
		filePath,
	}

	cmd := exec.Command("ffmpeg", args...)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg recording failed: %v", err)
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat file failed: %v", err)
	}

	rec := &Recording{
		DeviceID:  deviceID,
		FilePath:  filePath,
		StartTime: now.Format(time.RFC3339),
		EndTime:   time.Now().Format(time.RFC3339),
		Duration:  duration,
		FileSize:  fi.Size(),
		EventType: "manual",
	}
	m.store.AddRecording(rec)
	return rec, nil
}

// ============ Motion-Triggered Recording ============

// motionRecordings tracks active ffmpeg recording processes for motion detection
// key = deviceID, value = cancel function + metadata
type motionRec struct {
	cancel   context.CancelFunc
	filePath string
	startAt time.Time
}

// StartMotionRecording async — ffmpeg in background without -t (indefinite until cancelled)
func (m *FFmpegManager) StartMotionRecording(deviceID string) (*motionRec, error) {
	device := m.store.GetDevice(deviceID)
	if device == nil {
		return nil, fmt.Errorf("device %s not found", deviceID)
	}

	now := time.Now()
	dir := filepath.Join(m.dataDir, "videos", deviceID, now.Format("20060102"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create dir: %v", err)
	}

	filename := fmt.Sprintf("%s.mp4", now.Format("150405"))
	filePath := filepath.Join(dir, filename)
	inputURL := buildInputURL(device)

	args := []string{
		"-rtsp_transport", "tcp",
		"-i", inputURL,
		"-c", "copy",
		"-y",
		filePath,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("ffmpeg start: %v", err)
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[motion-rec-%s] %s", shortID(deviceID), scanner.Text())
		}
	}()

	go func() {
		cmd.Wait()
		// Record result after ffmpeg exits
		fi, err := os.Stat(filePath)
		if err == nil && fi.Size() > 0 {
			rec := &Recording{
				DeviceID:  deviceID,
				FilePath:  filePath,
				StartTime: now.Format(time.RFC3339),
				EndTime:   time.Now().Format(time.RFC3339),
				Duration:  int(time.Since(now).Seconds()),
				FileSize:  fi.Size(),
				EventType: "motion",
			}
			m.store.AddRecording(rec)
			log.Printf("[motion-rec-%s] saved: %s (%.1fMB)", shortID(deviceID), filePath, float64(fi.Size())/1e6)
		}
	}()

	return &motionRec{
		cancel:   cancel,
		filePath: filePath,
		startAt: now,
	}, nil
}

// ============ HTTP API ============

type Server struct {
	store    *Store
	ffmpeg   *FFmpegManager
	director *Director
	disco    *DiscoveryManager
	mux      *http.ServeMux
	dataDir  string
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
	frontendDir := s.dataDir + "/../frontend"
	if abs, err := filepath.Abs(frontendDir); err == nil {
		if _, err := os.Stat(abs); err == nil {
			fs := http.FileServer(http.Dir(abs))
			s.mux.Handle("/ui/", http.StripPrefix("/ui/", fs))
			s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" {
					http.Redirect(w, r, "/ui/login.html", 302)
					return
				}
				fp := filepath.Join(abs, r.URL.Path)
				if _, err := os.Stat(fp); err == nil {
					http.ServeFile(w, r, fp)
					return
				}
				s.error(w, "not found", 404)
			})
		} else {
			s.mux.HandleFunc("/", s.handleRoot)
		}
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
	s.mux.HandleFunc("/api/ptz/", s.handlePTZ)
	s.mux.HandleFunc("/api/motion/config", s.handleMotionConfig)
	s.mux.HandleFunc("/api/motion/config/", s.handleMotionConfigByDevice)
	s.mux.HandleFunc("/api/motion/events/", s.handleMotionEvents)
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
	default:
		s.error(w, "method not allowed", 405)
	}
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

	if err := s.ffmpeg.StartLiveProxyWithConfig(deviceID, cfg); err != nil {
		s.error(w, err.Error(), 500)
		return
	}
	s.json(w, map[string]interface{}{
		"status":   "started",
		"deviceId": deviceID,
		"hls_url":  fmt.Sprintf("http://192.0.2.107:8888/live/%s/index.m3u8", deviceID),
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
		"version":          "0.5.0",
		"devices":          devices,
		"online_devices":   onlineCount,
		"recordings":       recordings,
		"mediamtx_hls_url": "http://192.0.2.107:8888/",
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
		"version":          "0.5.0",
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
	defer m.mu.Unlock()

	now := time.Now()
	for id, p := range m.proxies {
		if !p.Running || p.Cmd == nil || p.Cmd.Process == nil || p.Cmd.ProcessState != nil {
			continue
		}
		if now.Sub(p.started) > 30*time.Minute {
			log.Printf("[health] proxy %s running for %.0f min", id[:8], now.Sub(p.started).Minutes())
		}
	}
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

// Update NewServer to start motion detector
func NewServer(store *Store, dataDir string) *Server {
	initAuth()
	s := &Server{
		store:    store,
		ffmpeg:   NewFFmpegManager(store, dataDir),
		director: NewDirector(NewFFmpegManager(store, dataDir), store, dataDir),
		disco:    NewDiscoveryManager(),
		mux:      http.NewServeMux(),
		dataDir:  dataDir,
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

	// Start motion detector
	go s.startMotionDetector()

	// Start schedule checker
	go s.startScheduler()
	// Start snapshot capture
	go s.startSnapshotter()

	return s
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
	log.Printf("=== 视频流管理平台 v0.1.0 ===")
	log.Printf("API 服务: http://0.0.0.0%s", addr)
	log.Printf("打开浏览器访问 http://localhost%s", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatal(err)
	}
}
