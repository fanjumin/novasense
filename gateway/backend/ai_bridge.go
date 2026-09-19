package main

// ai_bridge.go — AI sidecar(novasense-ai) 桥接层与"缝A/B"的支撑逻辑。
//
// 设计纪律(见 docs/AI-UPGRADE-PLAN.md):
//   - 本文件集中所有 AI 相关代码; main.go/store.go 仅各插 1-2 行调用(缝位埋点)。
//   - fail-open: sidecar 未配置/超时/非200/未就绪 → 返回 nil, 调用方走 v0.8.4 原行为。
//   - 三态开关: AI_MODE=off|shadow|enforce, shadow 只记录不拦截, enforce 有权降级背景误报。

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type aiConfig struct {
	URL     string
	Token   string
	Mode    string // off | shadow | enforce
	Timeout time.Duration
	Client  *http.Client
}

func newAIConfig() *aiConfig {
	cfg := &aiConfig{
		URL:   strings.TrimRight(os.Getenv("AI_SIDECAR_URL"), "/"),
		Token: os.Getenv("AI_INTERNAL_TOKEN"),
		Mode:  strings.ToLower(strings.TrimSpace(os.Getenv("AI_MODE"))),
	}
	switch cfg.Mode {
	case "off", "shadow", "enforce":
	case "":
		cfg.Mode = "shadow"
	default:
		log.Printf("[ai] unknown AI_MODE=%q, fallback to shadow", cfg.Mode)
		cfg.Mode = "shadow"
	}
	ms := 800
	if v, err := strconv.Atoi(os.Getenv("AI_TIMEOUT_MS")); err == nil && v > 50 {
		ms = v
	}
	cfg.Timeout = time.Duration(ms) * time.Millisecond
	cfg.Client = &http.Client{Timeout: cfg.Timeout}
	if cfg.URL == "" {
		cfg.Mode = "off"
		log.Printf("[ai] AI_SIDECAR_URL not set — AI seams disabled (pure v0.8.4 behavior)")
	} else {
		log.Printf("[ai] sidecar=%s mode=%s timeout=%dms", cfg.URL, cfg.Mode, ms)
	}
	return cfg
}

func (s *Server) aiEnabled() bool {
	return s.ai != nil && s.ai.URL != "" && s.ai.Mode != "off"
}

// ---- /v1/judge ----

type aiDetection struct {
	Cls  string    `json:"cls"`
	Conf float64   `json:"conf"`
	Box  []float64 `json:"box"`
}

type aiVerdict struct {
	HasTarget  bool
	Detections []aiDetection
	TopCls     string
	TopConf    float64
}

// aiJudge 送运动快照给 sidecar 判帧。任何失败返回 nil(弃权=fail-open)。
func (s *Server) aiJudge(imagePath, deviceID string) *aiVerdict {
	if !s.aiEnabled() {
		return nil
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"image_b64": base64.StdEncoding.EncodeToString(data),
		"device_id": deviceID,
		"tasks":     []string{"detect"},
	})
	return aiPost(s.ai, "/v1/judge", payload, func(body io.Reader) *aiVerdict {
		var r struct {
			HasTarget  bool          `json:"has_target"`
			Detections []aiDetection `json:"detections"`
			TopCls     string        `json:"top_class"`
			TopConf    float64       `json:"top_conf"`
		}
		if json.NewDecoder(body).Decode(&r) != nil {
			return nil
		}
		return &aiVerdict{HasTarget: r.HasTarget, Detections: r.Detections, TopCls: r.TopCls, TopConf: r.TopConf}
	})
}

// ---- /v1/embed_face ----

// aiEmbedFace 对人脸缩略图提 512d 向量; 失败/未就绪返回 nil。
func (s *Server) aiEmbedFace(imagePath string) []float32 {
	if !s.aiEnabled() {
		return nil
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"image_b64": base64.StdEncoding.EncodeToString(data),
	})
	return aiPost(s.ai, "/v1/embed_face", payload, func(body io.Reader) []float32 {
		var r struct {
			Embedding []float32 `json:"embedding"`
			Quality   float64   `json:"quality"`
		}
		if json.NewDecoder(body).Decode(&r) != nil || len(r.Embedding) == 0 {
			return nil
		}
		if r.Quality > 0 && r.Quality < 0.25 { // 低质量脸不提向量,交给 aHash 老路
			return nil
		}
		return r.Embedding
	})
}

// aiPost 通用请求: 非 200/网络错误一律返回零值(fail-open)。
func aiPost[T any](cfg *aiConfig, path string, payload []byte, parse func(io.Reader) T) T {
	var zero T
	req, err := http.NewRequest("POST", cfg.URL+path, bytes.NewReader(payload))
	if err != nil {
		return zero
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := cfg.Client.Do(req)
	if err != nil {
		log.Printf("[ai] %s fail-open: %v", path, err)
		return zero
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return zero
	}
	return parse(resp.Body)
}

// ---- embedding BLOB 编解码(小端 float32, 与 sidecar 数组序一致) ----

const aiEmbedModel = "arcface-r100-v1"

func aiFloat32ToBlob(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	buf := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func aiBlobToFloat32(b []byte) []float32 {
	if len(b) < 4 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func cosineSim(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na <= 0 || nb <= 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

// 匹配阈值(自采集标定见实施报告): ≥KNOWN 认定为已知; (REVIEW,KNOWN) 记分待复核。
const (
	aiFaceKnownThr  = 0.62
	aiFaceReviewThr = 0.45
)

// MatchFaceEmbedding 返回(命中, 余弦分, 库内是否存在可用向量)。
// 第三返回值为 false 时(冷启动/全库无向量), 调用方应回退 aHash 路径。
func MatchFaceEmbedding(emb []float32, known []*KnownFace) (*KnownFace, float64, bool) {
	best, bestFace := 0.0, (*KnownFace)(nil)
	hasVectors := false
	for _, kf := range known {
		e := aiBlobToFloat32(kf.Embedding)
		if len(e) == 0 {
			continue
		}
		hasVectors = true
		if sc := cosineSim(emb, e); sc > best {
			best, bestFace = sc, kf
		}
	}
	if bestFace != nil && best >= aiFaceKnownThr {
		return bestFace, best, true
	}
	return nil, best, hasVectors
}

// ---- 只读状态端点(GET /api/ai/status, 需登录) ----

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	out := map[string]interface{}{"enabled": s.aiEnabled(), "mode": s.ai.Mode, "sidecar": s.ai.URL}
	if s.aiEnabled() {
		req, _ := http.NewRequest("GET", s.ai.URL+"/v1/healthz", nil)
		if s.ai.Token != "" {
			req.Header.Set("Authorization", "Bearer "+s.ai.Token)
		}
		client := &http.Client{Timeout: s.ai.Timeout}
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var h map[string]interface{}
			if json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&h) == nil {
				out["sidecar"] = h
				out["reachable"] = true
			}
		}
	}
	s.json(w, out)
}
