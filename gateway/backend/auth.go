package main

// auth.go — 口令认证(bcrypt 持久哈希) + 强制改密模式 + 会话 + 登录防爆破。
//
// 修复对照(REMEDIATION-PLAN):
//   FIX-03 默认口令仅告警 → setup_required 模式: 口令仍为出厂默认时,除登录/设置接口外一律 403。
//   FIX-04 无盐 SHA-256   → bcrypt(cost 10) 持久化到 <DATA_DIR>/auth.json;旧部署无历史哈希可迁移
//                            (此前口令只在环境变量明文比对,auth.json 首启由 ADMIN_PASSWORD 或默认值生成)。
//   GAP-B /NetCamPro.apk 旧品牌免鉴权白名单 → 移除。
//   登录暴力防护: 按来源 IP 失败计数,5 次锁 600s(指数退避首轮)。

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// sessionDuration 定义在 main.go const 块(24h)。

var (
	authMu sync.Mutex
	// authState 持久化结构; LegacySHA256 仅用于兼容可能手写的 auth.json
	authState struct {
		BCryptHash string `json:"bcrypt_hash"`
	}
	setupRequired bool // 口令仍为出厂默认(admin) → true
	authFile      string
	sessions      = struct {
		mu sync.Mutex
		m  map[string]time.Time
	}{m: make(map[string]time.Time)}
	// 登录防爆破
	loginAttempts = struct {
		mu sync.Mutex
		m  map[string]*attempt // ip -> attempts
	}{m: make(map[string]*attempt)}
)

type attempt struct {
	fails        int
	blockedUntil time.Time
}

func initAuth(dataDir string) {
	authFile = dataDir + string(os.PathSeparator) + "auth.json"
	var envPw string
	if pw := os.Getenv("ADMIN_PASSWORD"); pw != "" {
		envPw = pw
	}

	authMu.Lock()
	defer authMu.Unlock()

	// 1) 读已持久化的哈希
	if b, err := os.ReadFile(authFile); err == nil {
		_ = json.Unmarshal(b, &authState)
		if authState.BCryptHash != "" {
			setupRequired = legacyIsDefaultHash(authState.BCryptHash)
			log.Printf("[auth] 口令已加载 (%s)", authFile)
			return
		}
	}
	// 2) 无哈希 → 由 env 或出厂默认生成
	pw := envPw
	if pw == "" {
		pw = "admin"
	}
	if h, err := bcrypt.GenerateFromPassword([]byte(pw), 10); err == nil {
		authState.BCryptHash = string(h)
		_ = persistAuth()
	}
	setupRequired = (pw == "admin")
	if setupRequired {
		log.Println("[auth] ⚠️ 正在使用出厂口令 'admin' — 已启用强制改密模式: 除登录与修改口令外,所有 API 返回 403")
	}
}

// legacyIsDefaultHash 检查存量哈希是否由出厂口令生成(sha256 兼容判定)。
func legacyIsDefaultHash(h string) bool {
	if len(h) == 64 { // 疑似旧 sha256 hex
		sum := sha256.Sum256([]byte("admin"))
		return strings.EqualFold(h, hex.EncodeToString(sum[:]))
	}
	if bcrypt.CompareHashAndPassword([]byte(h), []byte("admin")) == nil {
		return true
	}
	return false
}

func persistAuth() error {
	b, _ := json.MarshalIndent(authState, "", "  ")
	return os.WriteFile(authFile, b, 0o600)
}

// checkPassword 校验口令; 通过后若存量是旧 sha256(本版本不产生,防御兼容)则自动升级 bcrypt。
func checkPassword(pw string) bool {
	authMu.Lock()
	defer authMu.Unlock()
	if authState.BCryptHash == "" {
		return false
	}
	if len(authState.BCryptHash) == 64 { // 手写的旧格式: sha256 比对+升级
		sum := sha256.Sum256([]byte(pw))
		if !strings.EqualFold(authState.BCryptHash, hex.EncodeToString(sum[:])) {
			return false
		}
	} else if bcrypt.CompareHashAndPassword([]byte(authState.BCryptHash), []byte(pw)) != nil {
		return false
	}
	if h, err := bcrypt.GenerateFromPassword([]byte(pw), 10); err == nil {
		authState.BCryptHash = string(h)
		_ = persistAuth()
	}
	return true
}

// setPassword 校验策略并落库; 返回错误消息("" 表示成功)。
func setPassword(newPw string) string {
	if len(newPw) < 10 {
		return "口令长度不足 10 位"
	}
	for _, weak := range []string{"admin", "password", "1234567890", "novasense"} {
		if strings.Contains(strings.ToLower(newPw), weak) {
			return "口令包含常见弱词典根,请更换"
		}
	}
	authMu.Lock()
	defer authMu.Unlock()
	h, err := bcrypt.GenerateFromPassword([]byte(newPw), 10)
	if err != nil {
		return "hash 生成失败: " + err.Error()
	}
	authState.BCryptHash = string(h)
	if err := persistAuth(); err != nil {
		return "写入 auth.json 失败: " + err.Error()
	}
	setupRequired = false
	sessions.mu.Lock()
	sessions.m = make(map[string]time.Time) // 改密后全端登出
	sessions.mu.Unlock()
	log.Println("[auth] 管理员口令已更新, 强制改密模式解除")
	return ""
}

func requireSetup() bool {
	authMu.Lock()
	defer authMu.Unlock()
	return setupRequired
}

// ---- 登录防爆破 ----

const (
	maxFails   = 5
	lockWindow = 10 * time.Minute
)

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" { // 信任本机反代
		host = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	return host
}

// loginBlocked 返回 (是否封禁, 建议退避秒数)。
func loginBlocked(ip string) (bool, int) {
	loginAttempts.mu.Lock()
	defer loginAttempts.mu.Unlock()
	a := loginAttempts.m[ip]
	if a == nil {
		return false, 0
	}
	if time.Now().Before(a.blockedUntil) {
		return true, int(time.Until(a.blockedUntil).Seconds())
	}
	return false, 0
}

func loginFailed(ip string) {
	loginAttempts.mu.Lock()
	defer loginAttempts.mu.Unlock()
	a := loginAttempts.m[ip]
	if a == nil {
		a = &attempt{}
		loginAttempts.m[ip] = a
	}
	a.fails++
	if a.fails >= maxFails {
		backoff := lockWindow * time.Duration(1<<uint(a.fails-maxFails)) // 指数退避
		if backoff > time.Hour {
			backoff = time.Hour
		}
		a.blockedUntil = time.Now().Add(backoff)
		log.Printf("[auth] IP %s 连续失败 %d 次, 锁定至 %v", ip, a.fails, a.blockedUntil)
	}
}

func loginOK(ip string) {
	loginAttempts.mu.Lock()
	defer loginAttempts.mu.Unlock()
	delete(loginAttempts.m, ip)
}

// ---- 会话 ----

func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
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
	sessions.m[c.Value] = time.Now().Add(sessionDuration)
	return true
}

// PublicPaths — 免鉴权白名单。
// 强制改密模式下还会再收紧一层(setupBypassPaths),见 ServeHTTP。
var publicPaths = []string{
	"/api/login",
	"/api/setup/", // status + password (password 端点内部自校验仅 setup 模式可调)
	"/api/health",
	"/api/license/check",
	"/api/events",
	"/hls",
	"/ui/login.html",
}

func isPublicPath(path string) bool {
	for _, p := range publicPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
