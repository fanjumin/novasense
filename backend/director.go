package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ============ Director (导播系统) ============

type Director struct {
	mu        sync.Mutex
	sources   map[string]*DirectorSource
	pgm       *DirectorPGM
	overlays  []DirectorOverlay
	ffmpeg    *FFmpegManager
	store     *Store
	dataDir   string
}

type DirectorSource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`      // RTSP / HLS URL
	DeviceID string `json:"device_id"`
	Type     string `json:"type"`     // camera / video / black / image
}

type DirectorPGM struct {
	SourceID    string `json:"source_id"`
	Status      string `json:"status"`     // idle / live / transitioning
	OutputURL   string `json:"output_url"` // HLS output URL
	PushURL     string `json:"push_url"`   // RTMP push URL (optional)
	StartedAt   time.Time `json:"started_at"`
	ProgramPID  int       `json:"-"`       // FFmpeg process PID
	stopChan    chan struct{}
}

type DirectorOverlay struct {
	ID      string `json:"id"`
	Type    string `json:"type"`    // text / logo / pip
	Text    string `json:"text,omitempty"`
	Image   string `json:"image,omitempty"`  // path or URL
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	Enabled bool   `json:"enabled"`
	FontSize int   `json:"font_size,omitempty"`
	Color   string `json:"color,omitempty"`  // hex color
}

func NewDirector(ffmpeg *FFmpegManager, store *Store, dataDir string) *Director {
	return &Director{
		sources:  make(map[string]*DirectorSource),
		pgm:      &DirectorPGM{Status: "idle"},
		overlays: []DirectorOverlay{},
		ffmpeg:   ffmpeg,
		store:    store,
		dataDir:  dataDir,
	}
}

// SyncSources: load camera devices as director sources
func (d *Director) SyncSources() {
	d.mu.Lock()
	defer d.mu.Unlock()

	devices := d.store.ListDevices()

	newSources := make(map[string]*DirectorSource)
	// Keep existing director sources
	for _, dev := range devices {
		id := "cam_" + dev.ID
		if existing, ok := d.sources[id]; ok {
			newSources[id] = existing
			continue
		}
		newSources[id] = &DirectorSource{
			ID:       id,
			Name:     "🎥 " + dev.Name,
			URL:      dev.URL,
			DeviceID: dev.ID,
			Type:     "camera",
		}
	}

	// Add built-in sources
	newSources["black"] = &DirectorSource{ID: "black", Name: "⏹ 黑场", Type: "black"}
	newSources["test"] = &DirectorSource{ID: "test", Name: "📊 测试图", Type: "image"}

	d.sources = newSources
}

func (d *Director) GetSources() []*DirectorSource {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]*DirectorSource, 0, len(d.sources))
	for _, s := range d.sources {
		result = append(result, s)
	}
	return result
}

func (d *Director) GetStatus() map[string]interface{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return map[string]interface{}{
		"pgm":      d.pgm,
		"sources":  len(d.sources),
		"overlays": d.overlays,
	}
}

// SwitchTo: switch PGM to a source with optional transition
func (d *Director) SwitchTo(sourceID string, transition string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	source, ok := d.sources[sourceID]
	if !ok {
		return fmt.Errorf("source %s not found", sourceID)
	}

	// Stop current PGM
	if d.pgm.stopChan != nil {
		close(d.pgm.stopChan)
	}
	if d.pgm.ProgramPID > 0 {
		exec.Command("kill", fmt.Sprintf("%d", d.pgm.ProgramPID)).Run()
	}

	d.pgm.Status = "transitioning"
	d.pgm.SourceID = sourceID

	// Build input args
	inputArgs := d.buildInputArgs(source)
	if inputArgs == nil {
		d.pgm.Status = "idle"
		return fmt.Errorf("no input for source %s", sourceID)
	}

	// Build filter for overlays
	filterArgs := d.buildFilterArgs(sourceID)

	// Output: HLS to MediaMTX
	hlsURL := fmt.Sprintf("rtmp://127.0.0.1:1935/live/director")

	// Build full args
	args := []string{}
	args = append(args, inputArgs...)
	if len(filterArgs) > 0 {
		args = append(args, filterArgs...)
	} else {
		// If no filter, just pass through video
		args = append(args, "-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-r", "30")
	}
	args = append(args, "-c:a", "aac", "-ar", "44100", "-b:a", "128k", "-f", "flv", hlsURL)

	if d.pgm.PushURL != "" {
		// Dual output: HLS + push
		args = append(args, "-f", "tee",
			fmt.Sprintf("[f=flv]%s|[f=flv]%s", hlsURL, d.pgm.PushURL))
	}

	ffmpegBin := detectFFmpegPath()
	cmd := exec.Command(ffmpegBin, args...)
	if err := cmd.Start(); err != nil {
		d.pgm.Status = "idle"
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	d.pgm.ProgramPID = cmd.Process.Pid
	d.pgm.Status = "live"
	d.pgm.StartedAt = time.Now()
	d.pgm.OutputURL = "/live/director/index.m3u8"
	d.pgm.stopChan = make(chan struct{})

	// Monitor process
	go func() {
		cmd.Wait()
		d.mu.Lock()
		d.pgm.Status = "idle"
		d.pgm.ProgramPID = 0
		d.mu.Unlock()
	}()

	return nil
}

func (d *Director) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pgm.ProgramPID > 0 {
		exec.Command("kill", fmt.Sprintf("%d", d.pgm.ProgramPID)).Run()
	}
	if d.pgm.stopChan != nil {
		close(d.pgm.stopChan)
	}
	d.pgm.Status = "idle"
	d.pgm.SourceID = ""
	d.pgm.ProgramPID = 0
}

func (d *Director) SetOverlay(overlay DirectorOverlay) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i, o := range d.overlays {
		if o.ID == overlay.ID {
			d.overlays[i] = overlay
			return
		}
	}
	d.overlays = append(d.overlays, overlay)

	// If PGM is running, restart with new overlays
	if d.pgm.Status == "live" {
		go func() {
			_ = d.SwitchTo(d.pgm.SourceID, "cut")
		}()
	}
}

func (d *Director) RemoveOverlay(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, o := range d.overlays {
		if o.ID == id {
			d.overlays = append(d.overlays[:i], d.overlays[i+1:]...)
			break
		}
	}
}

func (d *Director) SetPushURL(url string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pgm.PushURL = url
}

func (d *Director) buildInputArgs(source *DirectorSource) []string {
	switch source.Type {
	case "camera":
		// Get the HLS URL from the live proxy
		return []string{"-i", source.URL, "-fflags", "nobuffer", "-flags", "low_delay"}
	case "black":
		return []string{"-f", "lavfi", "-i", "color=c=#1a1a2e:s=1920x1080:r=30", "-f", "lavfi", "-i", "anullsrc"}
	case "image":
		return []string{"-f", "lavfi", "-i", "color=c=#00E5FF:s=1920x1080:r=30,drawgrid=w=iw/20:h=ih/20:t=2:c=white@0.1", "-f", "lavfi", "-i", "anullsrc"}
	default:
		return nil
	}
}

func (d *Director) buildFilterArgs(sourceID string) []string {
	var filters []string
	hasVideoFilter := false
	audioLabel := "0:a"

	for _, o := range d.overlays {
		if !o.Enabled {
			continue
		}
		hasVideoFilter = true
		switch o.Type {
		case "text":
			if o.Color == "" {
				o.Color = "#FFFFFF"
			}
			if o.FontSize == 0 {
				o.FontSize = 24
			}
			escaped := strings.ReplaceAll(o.Text, "'", "'\\\\\\''")
			filters = append(filters,
				fmt.Sprintf("drawtext=text='%s':fontsize=%d:fontcolor=%s:x=%d:y=%d",
					escaped, o.FontSize, o.Color, o.X, o.Y))
		case "logo":
			filters = append(filters,
				fmt.Sprintf("movie='%s'[logo];[0:v][logo]overlay=%d:%d",
					o.Image, o.X, o.Y))
		case "pip":
			filters = append(filters,
				fmt.Sprintf("[0:v]scale=%d:%d[bg];[1:v]scale=%d:%d[pip];[bg][pip]overlay=%d:%d",
					1920, 1080, o.Width, o.Height, o.X, o.Y))
		}
	}

	if !hasVideoFilter {
		return nil
	}

	filterStr := strings.Join(filters, ",")
	return []string{"-filter_complex", filterStr, "-map", "[out]", "-map", audioLabel}
}

func detectFFmpegPath() string {
	// Common paths
	paths := []string{
		"/home/deployuser/projects/ffmpeg/ffmpeg-git-6.0-amd64-static/ffmpeg",
		"ffmpeg",
		"/usr/bin/ffmpeg",
	}
	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return "ffmpeg"
}

// ============ API Handlers ============

func (s *Server) handleDirector(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.json(w, s.director.GetStatus())
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleDirectorSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.director.SyncSources()
		s.json(w, s.director.GetSources())
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleDirectorSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	var req struct {
		SourceID   string `json:"source_id"`
		Transition string `json:"transition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.error(w, "invalid JSON", 400)
		return
	}
	if req.Transition == "" {
		req.Transition = "cut"
	}
	if err := s.director.SwitchTo(req.SourceID, req.Transition); err != nil {
		s.error(w, err.Error(), 500)
		return
	}
	s.json(w, map[string]string{"status": "switched", "source": req.SourceID})
}

func (s *Server) handleDirectorOverlays(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.json(w, s.director.overlays)
	case "POST":
		var overlay DirectorOverlay
		if err := json.NewDecoder(r.Body).Decode(&overlay); err != nil {
			s.error(w, "invalid JSON", 400)
			return
		}
		s.director.SetOverlay(overlay)
		s.json(w, map[string]string{"status": "ok"})
	case "DELETE":
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		s.director.RemoveOverlay(id)
		s.json(w, map[string]string{"status": "removed"})
	default:
		s.error(w, "method not allowed", 405)
	}
}

func (s *Server) handleDirectorStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "method not allowed", 405)
		return
	}
	s.director.Stop()
	s.json(w, map[string]string{"status": "stopped"})
}
