package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)
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
	"/api/events",
	"/hls",
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

