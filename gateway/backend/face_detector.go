package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"math"
	"os"
	"sync"

	pigo "github.com/esimov/pigo/core"
)

// ============ Face Data Models ============

// KnownFace — a face the user has named (persisted in DB)
type KnownFace struct {
	ID         string `json:"id"`
	Label      string `json:"label"`      // user-given name, e.g. "张三"
	FaceHash   string `json:"face_hash"`  // average hash (64-bit hex)
	DeviceID   string `json:"device_id"`  // which device first saw this face
	ThumbPath  string `json:"thumb_path"` // cropped face thumbnail
	CreatedAt  string `json:"created_at"`
	LastSeenAt string `json:"last_seen_at"`
	SeenCount  int    `json:"seen_count"`
	Embedding  []byte `json:"-"`                   // 缝B: 512d float32 小端 BLOB
	EmbModel   string `json:"emb_model,omitempty"` // 缝B: 向量模型标识(换模型须全库重算)
}

// FaceEvent — a face detection event linked to a motion event
type FaceEvent struct {
	ID            string  `json:"id"`
	MotionEventID string  `json:"motion_event_id"`
	DeviceID      string  `json:"device_id"`
	FaceID        string  `json:"face_id"`    // matched KnownFace.ID, empty if unknown
	Label         string  `json:"label"`      // resolved label or "unknown"
	Confidence    float64 `json:"confidence"` // 0.0–1.0 match confidence
	Score         float64 `json:"score"`      // pigo detection score
	Bounds        string  `json:"bounds"`     // JSON: {x,y,w,h}
	ThumbPath     string  `json:"thumb_path"` // cropped face thumbnail file
	DetectedAt    string  `json:"detected_at"`
	CreatedAt     string  `json:"created_at"`
}

// ============ Face Detector ============

type FaceDetector struct {
	mu         sync.Mutex
	pigo       *pigo.Pigo
	classifier *pigo.Pigo
	cascade    []byte
	angle      float64
	ready      bool
}

// FaceResult — single face found in an image
type FaceResult struct {
	Row    int     // center row
	Col    int     // center col
	Scale  int     // face size
	Score  float64 // detection confidence
	Bounds image.Rectangle
}

// NewFaceDetector creates a face detector from the cascade file.
// The cascade file is embedded at compile time or loaded from the filesystem.
func NewFaceDetector(cascadePath string) *FaceDetector {
	fd := &FaceDetector{angle: 0.0}

	cascadeFile, err := os.ReadFile(cascadePath)
	if err != nil {
		log.Printf("[face] ⚠️ cascade file not found at %s: %v (face detection disabled)", cascadePath, err)
		return fd
	}
	fd.cascade = cascadeFile

	p := pigo.NewPigo()
	classifier, err := p.Unpack(cascadeFile)
	if err != nil {
		log.Printf("[face] ⚠️ cascade unpack failed: %v", err)
		return fd
	}
	fd.classifier = classifier
	fd.ready = true
	log.Printf("[face] detector ready (cascade: %s, %d bytes)", cascadePath, len(cascadeFile))
	return fd
}

// Ready returns true if the detector was initialized successfully.
func (fd *FaceDetector) Ready() bool {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	return fd.ready
}

// DetectFaces finds faces in a JPEG image at the given path.
// Returns a list of face regions, sorted by score descending.
func (fd *FaceDetector) DetectFaces(imagePath string) ([]FaceResult, error) {
	if !fd.Ready() {
		return nil, fmt.Errorf("face detector not ready")
	}

	src, err := pigo.GetImage(imagePath)
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}

	pixels := pigo.RgbToGrayscale(src)
	bounds := src.Bounds()
	cols, rows := bounds.Max.X, bounds.Max.Y

	// Run cascade at multiple sizes
	cParams := pigo.CascadeParams{
		MinSize:     40,  // minimum face size (pixels)
		MaxSize:     400, // maximum face size
		ShiftFactor: 0.1, // step size (smaller = more accurate but slower)
		ScaleFactor: 1.1, // scale ratio between passes
		ImageParams: pigo.ImageParams{
			Pixels: pixels,
			Rows:   rows,
			Cols:   cols,
			Dim:    cols,
		},
	}

	dets := fd.classifier.RunCascade(cParams, fd.angle)

	// Cluster overlapping detections (IoU threshold 0.2)
	dets = fd.classifier.ClusterDetections(dets, 0.2)

	if len(dets) == 0 {
		return nil, nil
	}

	results := make([]FaceResult, 0, len(dets))
	for _, det := range dets {
		faceSize := det.Scale
		r := image.Rect(
			det.Col-faceSize/2,
			det.Row-faceSize/2,
			det.Col+faceSize/2,
			det.Row+faceSize/2,
		)
		// Clamp to image bounds
		r = r.Intersect(bounds)
		if r.Empty() {
			continue
		}
		results = append(results, FaceResult{
			Row:    det.Row,
			Col:    det.Col,
			Scale:  det.Scale,
			Score:  float64(det.Q),
			Bounds: r,
		})
	}
	return results, nil
}

// CropFace extracts the face region from a JPEG image as a new image.
func CropFace(imagePath string, bounds image.Rectangle) (image.Image, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := jpeg.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode JPEG: %w", err)
	}

	// Sub-image
	type subImage interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := img.(subImage); ok {
		return si.SubImage(bounds), nil
	}

	// Fallback: manual crop
	cropped := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			cropped.Set(x-bounds.Min.X, y-bounds.Min.Y, img.At(x, y))
		}
	}
	return cropped, nil
}

// SaveFaceThumbnail saves a face crop as a JPEG thumbnail. Returns the file path.
func SaveFaceThumbnail(faceImg image.Image, thumbDir string) (string, error) {
	os.MkdirAll(thumbDir, 0755)
	f, err := os.CreateTemp(thumbDir, "face_*.jpg")
	if err != nil {
		return "", fmt.Errorf("create thumb file: %w", err)
	}
	defer f.Close()

	err = jpeg.Encode(f, faceImg, &jpeg.Options{Quality: 85})
	if err != nil {
		return "", fmt.Errorf("encode thumb: %w", err)
	}
	return f.Name(), nil
}

// ============ Face Hashing (Average Hash) ============

// AverageHash computes a 64-bit average hash (aHash) for an image.
// 1. Resize to 8x8 grayscale
// 2. Compute average pixel value
// 3. Each pixel → 1 if >= average, 0 otherwise
// Returns hex-encoded 16-char string.
func AverageHash(img image.Image) string {
	// Resize to 8x8 using nearest-neighbor
	resized := resizeNearest(img, 8, 8)

	// Convert to grayscale and compute average
	var pixels [64]float64
	var sum float64
	idx := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			r, g, b, _ := resized.At(x, y).RGBA()
			gray := 0.299*float64(r/257) + 0.587*float64(g/257) + 0.114*float64(b/257)
			pixels[idx] = gray
			sum += gray
			idx++
		}
	}
	avg := sum / 64.0

	// Build 64-bit hash
	var hash uint64
	for i, p := range pixels {
		if p >= avg {
			hash |= 1 << uint(i)
		}
	}
	return fmt.Sprintf("%016x", hash)
}

// HammingDistance computes the number of differing bits between two hex hashes.
func HammingDistance(hash1, hash2 string) int {
	// Parse hex strings
	var h1, h2 uint64
	fmt.Sscanf(hash1, "%016x", &h1)
	fmt.Sscanf(hash2, "%016x", &h2)
	xor := h1 ^ h2
	// Popcount
	count := 0
	for xor != 0 {
		xor &= xor - 1
		count++
	}
	return count
}

// MatchFace finds the closest matching known face by Hamming distance.
// Returns the match and confidence (0.0–1.0). If no match within threshold, returns nil.
func MatchFace(faceHash string, knownFaces []*KnownFace, threshold int) (*KnownFace, float64) {
	if len(knownFaces) == 0 {
		return nil, 0
	}
	bestDist := 999
	var bestFace *KnownFace
	for _, kf := range knownFaces {
		dist := HammingDistance(faceHash, kf.FaceHash)
		if dist < bestDist {
			bestDist = dist
			bestFace = kf
		}
	}
	if bestFace == nil || bestDist > threshold {
		return nil, 0
	}
	// Confidence: 1.0 at 0 distance, decays linearly to 0.1 at threshold
	confidence := 1.0 - float64(bestDist)/float64(threshold)
	if confidence < 0.1 {
		confidence = 0.1
	}
	return bestFace, confidence
}

// ============ Image Util ============

// resizeNearest resizes an image to the target dimensions using nearest-neighbor.
func resizeNearest(src image.Image, dstW, dstH int) image.Image {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for dy := 0; dy < dstH; dy++ {
		for dx := 0; dx < dstW; dx++ {
			sx := int(math.Round(float64(dx) * float64(srcW) / float64(dstW)))
			sy := int(math.Round(float64(dy) * float64(srcH) / float64(dstH)))
			if sx >= srcW {
				sx = srcW - 1
			}
			if sy >= srcH {
				sy = srcH - 1
			}
			dst.Set(dx, dy, src.At(bounds.Min.X+sx, bounds.Min.Y+sy))
		}
	}
	return dst
}

// rgbaToGrayscaleString converts an RGBA image to a comma-separated grayscale string
// used for display preview (lightweight serialization).
func grayscaleString(img image.Image) string {
	// Average of RGB components at center pixel
	b := img.Bounds()
	midX := (b.Min.X + b.Max.X) / 2
	midY := (b.Min.Y + b.Max.Y) / 2
	r, g, bVal, _ := img.At(midX, midY).RGBA()
	gray := (0.299*float64(r/257) + 0.587*float64(g/257) + 0.114*float64(bVal/257))
	return fmt.Sprintf("%.0f", gray)
}
