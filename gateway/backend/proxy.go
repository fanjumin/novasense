package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)
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
	healthFails map[string]int        // consecutive health check failures
}

type FFmpegProc struct {
	ID      string
	Cmd     *exec.Cmd
	PullCmd *exec.Cmd // wapa-pull helper (WAPA protocol only)
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
		wapaBin:    filepath.Join(filepath.Dir(os.Args[0]), "bin", "wapa-pull"),
		healthFails: make(map[string]int),
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
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Bitrate     string `json:"bitrate"`
	VideoFilter string `json:"video_filter,omitempty"` // FFmpeg -vf chain, e.g. fisheye unwarp
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

	// Quick ffprobe to detect native resolution with 3s hard timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []string{
		"-rtsp_transport", "tcp",
		"-stimeout", "2000000",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0",
	}
	// For HTTP MJPEG, force input format before other args
	if strings.HasPrefix(device.URL, "http://") || strings.HasPrefix(device.URL, "https://") {
		args = append([]string{"-f", "mjpeg"}, args...)
	}
	args = append(args, device.URL)

	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[proxy] ffprobe探测失败或超时(ID=%s err=%v stderr=%s) 使用默认分辨率1920x1080",
			shortID(deviceID), err, strings.TrimSpace(stderr.String()))
		return ProxyConfig{Width: 1920, Height: 1080, Bitrate: "8000k"}
	}

	// Parse "1920,1080" from csv output
	out := strings.TrimSpace(stdout.String())
	parts := strings.Split(out, ",")
	if len(parts) != 2 {
		log.Printf("[proxy] ffprobe输出格式异常(ID=%s out=%s) 使用默认分辨率1920x1080",
			shortID(deviceID), out)
		return ProxyConfig{Width: 1920, Height: 1080, Bitrate: "8000k"}
	}

	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		log.Printf("[proxy] ffprobe解析失败(ID=%s w=%d h=%d) 使用默认分辨率1920x1080",
			shortID(deviceID), w, h)
		return ProxyConfig{Width: 1920, Height: 1080, Bitrate: "8000k"}
	}

	// Choose bitrate based on resolution
	var br string
	switch {
	case w >= 3840:
		br = "12000k"
	case w >= 2560:
		br = "6000k"
	case w >= 1920:
		br = "8000k"
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
		br = "8000k"
	}

	args := []string{
		"-fflags", "+genpts+nobuffer+flush_packets",
		"-flags", "low_delay",
	}

	// HTTP MJPEG: force input format + wallclock timestamps before -i
	if strings.HasPrefix(inputURL, "http://") || strings.HasPrefix(inputURL, "https://") {
		args = append(args, "-f", "mjpeg", "-use_wallclock_as_timestamps", "1")
	}

	// Input source (handles WAPA pipe vs URL vs device)
	if strings.HasPrefix(inputURL, "wapa://") {
		args = append(args, "-f", "m4v", "-err_detect", "ignore_err", "-i", "pipe:0")
	} else {
		args = append(args, "-i", inputURL)
	}

	// Optional video filter chain (e.g. fisheye unwarp) applied after decode
	if cfg.VideoFilter != "" {
		args = append(args, "-vf", cfg.VideoFilter)
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-b:v", br,
		"-maxrate", br,
		"-bufsize", br,
		"-s", fmt.Sprintf("%dx%d", w, h),
		"-r", "25",
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

	// Add RTSP-specific flags only for RTSP URLs (prepend so order stays correct)
	if strings.HasPrefix(inputURL, "rtsp://") || strings.HasPrefix(inputURL, "rtsps://") {
		// TCP transport for reliability through firewalls.
		args = append([]string{"-rtsp_transport", "tcp"}, args...)
	}
	// For USB cameras (/dev/video*), add input format
	if strings.HasPrefix(inputURL, "/dev/video") {
		args = append([]string{"-f", "v4l2", "-input_format", "mjpeg", "-framerate", "15"}, args...)
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

	// Fisheye unwarp is handled by the frontend (panorama.html, equisolid model).
	// Backend keeps the raw stream; explicit VideoFilter (if ever set) is applied
	// by the generic path above. The old v360 auto-injection is removed because
	// the equirect assumption does not match this lens (equisolid verified).

	m.mu.Lock()
	if p, ok := m.proxies[deviceID]; ok && p.Running {
		m.mu.Unlock()
		return nil // already running
	}
	m.mu.Unlock()

	// Acquire semaphore slot (blocks if at maxConcurrentFFmpeg)
	m.sem <- struct{}{}

	inputURL := buildInputURL(device)

	// Kill any orphaned ffmpeg process that may still be holding this device.
	// This handles the case where a previous backend instance was killed,
	// leaving behind zombie ffmpeg processes that block device access.
	if strings.HasPrefix(inputURL, "/dev/video") {
		// USB camera: use fuser to kill all processes holding the device
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		killCmd := exec.CommandContext(ctx, "fuser", "-k", inputURL)
		killCmd.CombinedOutput()
		cancel()
	} else if strings.HasPrefix(inputURL, "http://") || strings.HasPrefix(inputURL, "https://") {
		// HTTP/MJPEG camera: search for ffmpeg processes using the same URL and kill them
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		killCmd := exec.CommandContext(ctx, "sh", "-c",
			fmt.Sprintf("ps aux | grep 'ffmpeg.*-i %s' | grep -v grep | awk '{print $2}' | xargs -r kill -9 2>/dev/null", inputURL))
		killCmd.CombinedOutput()
		cancel()
	}

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
	var pullCmd *exec.Cmd
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
		pullCmd = exec.CommandContext(ctx, m.wapaBin, ipPort)
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

	// Run ffmpeg in its own process group so it survives backend restarts
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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
	if device.Protocol == "wapa" {
		proc.PullCmd = pullCmd
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
				// Kill old wapa-pull helper before restarting (prevents zombie accumulation)
				if proc.PullCmd != nil && proc.PullCmd.Process != nil {
					proc.PullCmd.Process.Kill()
				}
				os.Remove(filepath.Join(os.TempDir(), fmt.Sprintf("wapa-%s.fifo", deviceID)))
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

	// Kill the wapa-pull helper first (if any)
	if p.PullCmd != nil && p.PullCmd.Process != nil {
		p.PullCmd.Process.Kill()
	}

	// Clean up wapa FIFO
	fifoPath := filepath.Join(os.TempDir(), fmt.Sprintf("wapa-%s.fifo", deviceID))
	os.Remove(fifoPath)

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
