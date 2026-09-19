package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// ============ Data Models ============

type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	URL        string `json:"url"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Status     string `json:"status"`
	GroupName  string `json:"group_name"`
	CreatedAt  string `json:"created_at"`
	Capability string `json:"capability,omitempty"` // JSON blob from phone self-check
}

type Recording struct {
	ID        string `json:"id"`
	DeviceID  string `json:"device_id"`
	FilePath  string `json:"file_path"`
	PlayURL   string `json:"play_url"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Duration  int    `json:"duration"`
	FileSize  int64  `json:"file_size"`
	EventType string `json:"event_type"`
	CreatedAt string `json:"created_at"`
}

type Schedule struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DeviceID      string `json:"device_id"`
	TimeStart     string `json:"time_start"`
	TimeEnd       string `json:"time_end"`
	Days          string `json:"days"`
	DurationMin   int    `json:"duration_min"`
	Enabled       bool   `json:"enabled"`
	LastTriggered string `json:"last_triggered"`
	CreatedAt     string `json:"created_at"`
}

type SnapshotConfig struct {
	ID        string `json:"id"`
	DeviceID  string `json:"device_id"`
	Enabled   bool   `json:"enabled"`
	Interval  int    `json:"interval"`
	Retention int    `json:"retention"`
}

type MotionConfig struct {
	ID          string  `json:"id"`
	DeviceID    string  `json:"device_id"`
	Enabled     bool    `json:"enabled"`
	Sensitivity float64 `json:"sensitivity"`
	CooldownSec int     `json:"cooldown_sec"`
}

type MotionEvent struct {
	ID           string  `json:"id"`
	DeviceID     string  `json:"device_id"`
	DetectedAt   string  `json:"detected_at"`
	SnapshotPath string  `json:"snapshot_path"`
	CreatedAt    string  `json:"created_at"`
	AIClass      string  `json:"ai_class,omitempty"`     // 缝A: sidecar 判定类别(""=未判/弃权)
	AIConf       float64 `json:"ai_conf,omitempty"`      // 缝A: 置信度
	VerdictMode  string  `json:"verdict_mode,omitempty"` // 缝A: 判定时的开关(shadow/enforce)
}

// ============ SQLite Store ============

type Store struct {
	mu        sync.RWMutex
	db        *sql.DB
	videosDir string
	dataDir   string
}

func NewStore(videosDir, dataDir string) *Store {
	dbPath := filepath.Join(dataDir, "store.db")
	os.MkdirAll(dataDir, 0755)

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("[store] open db failed: %v", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{
		db:        db,
		videosDir: videosDir,
		dataDir:   dataDir,
	}
	s.migrate()
	return s
}

func (s *Store) migrate() {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			protocol TEXT NOT NULL DEFAULT 'rtsp',
			url TEXT NOT NULL DEFAULT '',
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'offline',
			group_name TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS recordings (
			id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL DEFAULT '',
			file_path TEXT NOT NULL DEFAULT '',
			play_url TEXT NOT NULL DEFAULT '',
			start_time TEXT NOT NULL DEFAULT '',
			end_time TEXT NOT NULL DEFAULT '',
			duration INTEGER NOT NULL DEFAULT 0,
			file_size INTEGER NOT NULL DEFAULT 0,
			event_type TEXT NOT NULL DEFAULT 'manual',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS schedules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			device_id TEXT NOT NULL DEFAULT '',
			time_start TEXT NOT NULL DEFAULT '',
			time_end TEXT NOT NULL DEFAULT '',
			days TEXT NOT NULL DEFAULT '',
			duration_min INTEGER NOT NULL DEFAULT 30,
			enabled INTEGER NOT NULL DEFAULT 1,
			last_triggered TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS snapshot_configs (
			id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			interval_sec INTEGER NOT NULL DEFAULT 30,
			retention_days INTEGER NOT NULL DEFAULT 7
		)`,
		`CREATE TABLE IF NOT EXISTS motion_configs (
			id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 0,
			sensitivity REAL NOT NULL DEFAULT 0.3,
			cooldown_sec INTEGER NOT NULL DEFAULT 30
		)`,
		`CREATE TABLE IF NOT EXISTS motion_events (
			id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL DEFAULT '',
			detected_at TEXT NOT NULL DEFAULT '',
			snapshot_path TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS licenses (
			key TEXT PRIMARY KEY,
			is_pro INTEGER NOT NULL DEFAULT 0,
			device_id TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS faces (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL DEFAULT 'unknown',
			face_hash TEXT NOT NULL DEFAULT '',
			device_id TEXT NOT NULL DEFAULT '',
			thumb_path TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			last_seen_at TEXT NOT NULL DEFAULT '',
			seen_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS face_events (
			id TEXT PRIMARY KEY,
			motion_event_id TEXT NOT NULL DEFAULT '',
			device_id TEXT NOT NULL DEFAULT '',
			face_id TEXT NOT NULL DEFAULT '',
			label TEXT NOT NULL DEFAULT 'unknown',
			confidence REAL NOT NULL DEFAULT 0.0,
			score REAL NOT NULL DEFAULT 0.0,
			bounds TEXT NOT NULL DEFAULT '',
			thumb_path TEXT NOT NULL DEFAULT '',
			detected_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS retention_config (
			id TEXT PRIMARY KEY,
			retention_days INTEGER NOT NULL DEFAULT 30,
			max_disk_usage_pct REAL NOT NULL DEFAULT 80.0,
			enabled INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
	}

	for _, ddl := range tables {
		if _, err := s.db.Exec(ddl); err != nil {
			log.Fatalf("[store] migrate failed: %v", err)
		}
	}
	// 运行期加列(AI 缝A/B): 列已存在报 duplicate column, 忽略 — 与 devices.capability 先例同法
	for _, alt := range []string{
		"ALTER TABLE motion_events ADD COLUMN ai_class TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE motion_events ADD COLUMN ai_conf REAL NOT NULL DEFAULT 0",
		"ALTER TABLE motion_events ADD COLUMN verdict_mode TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE faces ADD COLUMN embedding BLOB",
		"ALTER TABLE faces ADD COLUMN emb_model TEXT NOT NULL DEFAULT ''",
	} {
		if _, err := s.db.Exec(alt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			log.Printf("[store] migrate alter failed: %v", err)
		}
	}
	log.Println("[store] database migrated")
}

// ============ Devices ============

func (s *Store) ListDevices() []*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id, name, protocol, url, username, password, status, group_name, created_at, IFNULL(capability,'') FROM devices ORDER BY created_at DESC")
	if err != nil {
		log.Printf("[store] list devices: %v", err)
		return nil
	}
	defer rows.Close()
	var result []*Device
	for rows.Next() {
		d := &Device{}
		rows.Scan(&d.ID, &d.Name, &d.Protocol, &d.URL, &d.Username, &d.Password, &d.Status, &d.GroupName, &d.CreatedAt, &d.Capability)
		result = append(result, d)
	}
	return result
}

func (s *Store) GetDevice(id string) *Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d := &Device{}
	err := s.db.QueryRow("SELECT id, name, protocol, url, username, password, status, group_name, created_at, IFNULL(capability,'') FROM devices WHERE id=?", id).
		Scan(&d.ID, &d.Name, &d.Protocol, &d.URL, &d.Username, &d.Password, &d.Status, &d.GroupName, &d.CreatedAt, &d.Capability)
	if err != nil {
		return nil
	}
	return d
}

func (s *Store) AddDevice(d *Device) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d.ID = uuid.NewString()[:8]
	d.Status = "offline"
	d.CreatedAt = time.Now().Format(time.RFC3339)
	s.db.Exec("INSERT INTO devices (id, name, protocol, url, username, password, status, group_name, created_at) VALUES (?,?,?,?,?,?,?,?,?)",
		d.ID, d.Name, d.Protocol, d.URL, d.Username, d.Password, d.Status, d.GroupName, d.CreatedAt)
}

func (s *Store) UpdateDeviceStatus(id, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("UPDATE devices SET status=? WHERE id=?", status, id)
}

func (s *Store) DeleteDevice(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("DELETE FROM devices WHERE id=?", id)
}

// UpdateDeviceGroup updates the group_name for a device.
func (s *Store) UpdateDeviceGroup(id, groupName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("UPDATE devices SET group_name=? WHERE id=?", groupName, id)
}

// ListGroups returns all distinct group names with device counts.
type GroupInfo struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (s *Store) ListGroups() []*GroupInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT IFNULL(group_name,'未分组'), COUNT(*) FROM devices GROUP BY group_name ORDER BY group_name")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*GroupInfo
	for rows.Next() {
		g := &GroupInfo{}
		rows.Scan(&g.Name, &g.Count)
		result = append(result, g)
	}
	if result == nil {
		result = []*GroupInfo{}
	}
	return result
}

// RenameGroup changes all devices with oldGroup to newGroup.
func (s *Store) RenameGroup(oldGroup, newGroup string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("UPDATE devices SET group_name=? WHERE group_name=?", newGroup, oldGroup)
}

// SetDeviceCapability — stores JSON capability blob on a device record
func (s *Store) SetDeviceCapability(deviceID string, capability map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Try to add capability column if not exists (idempotent)
	s.db.Exec("ALTER TABLE devices ADD COLUMN capability TEXT DEFAULT ''")
	b, _ := json.Marshal(capability)
	s.db.Exec("UPDATE devices SET capability=? WHERE id=?", string(b), deviceID)
}

// ============ Recordings ============

func (s *Store) ListRecordings(deviceID string) []*Recording {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows *sql.Rows
	var err error
	if deviceID == "" {
		rows, err = s.db.Query("SELECT id, device_id, file_path, play_url, start_time, end_time, duration, file_size, event_type, created_at FROM recordings ORDER BY created_at DESC")
	} else {
		rows, err = s.db.Query("SELECT id, device_id, file_path, play_url, start_time, end_time, duration, file_size, event_type, created_at FROM recordings WHERE device_id=? ORDER BY created_at DESC", deviceID)
	}
	if err != nil {
		return []*Recording{}
	}
	defer rows.Close()
	result := []*Recording{}
	for rows.Next() {
		r := &Recording{}
		rows.Scan(&r.ID, &r.DeviceID, &r.FilePath, &r.PlayURL, &r.StartTime, &r.EndTime, &r.Duration, &r.FileSize, &r.EventType, &r.CreatedAt)
		if r.PlayURL == "" && r.FilePath != "" {
			r.PlayURL = makePlayURL(r.FilePath, s.videosDir)
		}
		result = append(result, r)
	}
	return result
}

func (s *Store) AddRecording(r *Recording) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = uuid.NewString()[:8]
	r.CreatedAt = time.Now().Format(time.RFC3339)
	r.PlayURL = makePlayURL(r.FilePath, s.videosDir)
	s.db.Exec("INSERT INTO recordings (id, device_id, file_path, play_url, start_time, end_time, duration, file_size, event_type, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)",
		r.ID, r.DeviceID, r.FilePath, r.PlayURL, r.StartTime, r.EndTime, r.Duration, r.FileSize, r.EventType, r.CreatedAt)
}

func makePlayURL(filePath, videosDir string) string {
	rel := strings.TrimPrefix(filePath, videosDir)
	return "/videos" + rel
}

// ============ Schedules ============

func (s *Store) ListSchedules() []*Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id, name, device_id, time_start, time_end, days, duration_min, enabled, last_triggered, created_at FROM schedules ORDER BY created_at DESC")
	if err != nil {
		return []*Schedule{}
	}
	defer rows.Close()
	result := []*Schedule{}
	for rows.Next() {
		sc := &Schedule{}
		var enabledInt int
		rows.Scan(&sc.ID, &sc.Name, &sc.DeviceID, &sc.TimeStart, &sc.TimeEnd, &sc.Days, &sc.DurationMin, &enabledInt, &sc.LastTriggered, &sc.CreatedAt)
		sc.Enabled = enabledInt == 1
		result = append(result, sc)
	}
	return result
}

func (s *Store) GetSchedule(id string) *Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sc := &Schedule{}
	var enabledInt int
	err := s.db.QueryRow("SELECT id, name, device_id, time_start, time_end, days, duration_min, enabled, last_triggered, created_at FROM schedules WHERE id=?", id).
		Scan(&sc.ID, &sc.Name, &sc.DeviceID, &sc.TimeStart, &sc.TimeEnd, &sc.Days, &sc.DurationMin, &enabledInt, &sc.LastTriggered, &sc.CreatedAt)
	if err != nil {
		return nil
	}
	sc.Enabled = enabledInt == 1
	return sc
}

func (s *Store) AddSchedule(sc *Schedule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc.ID = uuid.NewString()[:8]
	sc.CreatedAt = time.Now().Format(time.RFC3339)
	enabledInt := 0
	if sc.Enabled {
		enabledInt = 1
	}
	s.db.Exec("INSERT INTO schedules (id, name, device_id, time_start, time_end, days, duration_min, enabled, last_triggered, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)",
		sc.ID, sc.Name, sc.DeviceID, sc.TimeStart, sc.TimeEnd, sc.Days, sc.DurationMin, enabledInt, sc.LastTriggered, sc.CreatedAt)
}

func (s *Store) UpdateSchedule(sc *Schedule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enabledInt := 0
	if sc.Enabled {
		enabledInt = 1
	}
	s.db.Exec("UPDATE schedules SET name=?, device_id=?, time_start=?, time_end=?, days=?, duration_min=?, enabled=?, last_triggered=? WHERE id=?",
		sc.Name, sc.DeviceID, sc.TimeStart, sc.TimeEnd, sc.Days, sc.DurationMin, enabledInt, sc.LastTriggered, sc.ID)
}

func (s *Store) DeleteSchedule(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("DELETE FROM schedules WHERE id=?", id)
}

// ============ Snapshot Configs ============

func (s *Store) ListSnapshotConfigs() []*SnapshotConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id, device_id, enabled, interval_sec, retention_days FROM snapshot_configs")
	if err != nil {
		return []*SnapshotConfig{}
	}
	defer rows.Close()
	result := []*SnapshotConfig{}
	for rows.Next() {
		sn := &SnapshotConfig{}
		var enabledInt int
		rows.Scan(&sn.ID, &sn.DeviceID, &enabledInt, &sn.Interval, &sn.Retention)
		sn.Enabled = enabledInt == 1
		result = append(result, sn)
	}
	return result
}

func (s *Store) GetSnapshotConfig(id string) *SnapshotConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sn := &SnapshotConfig{}
	var enabledInt int
	err := s.db.QueryRow("SELECT id, device_id, enabled, interval_sec, retention_days FROM snapshot_configs WHERE id=?", id).
		Scan(&sn.ID, &sn.DeviceID, &enabledInt, &sn.Interval, &sn.Retention)
	if err != nil {
		return nil
	}
	sn.Enabled = enabledInt == 1
	return sn
}

func (s *Store) AddSnapshotConfig(sn *SnapshotConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sn.ID = uuid.NewString()[:8]
	enabledInt := 0
	if sn.Enabled {
		enabledInt = 1
	}
	s.db.Exec("INSERT INTO snapshot_configs (id, device_id, enabled, interval_sec, retention_days) VALUES (?,?,?,?,?)",
		sn.ID, sn.DeviceID, enabledInt, sn.Interval, sn.Retention)
}

func (s *Store) UpdateSnapshotConfig(sn *SnapshotConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	enabledInt := 0
	if sn.Enabled {
		enabledInt = 1
	}
	s.db.Exec("UPDATE snapshot_configs SET enabled=?, interval_sec=?, retention_days=? WHERE id=?",
		enabledInt, sn.Interval, sn.Retention, sn.ID)
}

func (s *Store) DeleteSnapshotConfig(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("DELETE FROM snapshot_configs WHERE id=?", id)
}

// ============ Export / Import ============

// ExportAll exports all data as JSON for backup
func (s *Store) ExportAll() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type exportData struct {
		Devices    []*Device         `json:"devices"`
		Recordings []*Recording      `json:"recordings"`
		Schedules  []*Schedule       `json:"schedules"`
		Snapshots  []*SnapshotConfig `json:"snapshots"`
		ExportedAt string            `json:"exported_at"`
	}

	data := exportData{
		Devices:    s.ListDevices(),
		Recordings: s.ListRecordings(""),
		Schedules:  s.ListSchedules(),
		Snapshots:  s.ListSnapshotConfigs(),
		ExportedAt: time.Now().Format(time.RFC3339),
	}

	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b)
}

// ImportAll imports data from JSON backup
func (s *Store) ImportAll(jsonData string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var data struct {
		Devices    []*Device         `json:"devices"`
		Recordings []*Recording      `json:"recordings"`
		Schedules  []*Schedule       `json:"schedules"`
		Snapshots  []*SnapshotConfig `json:"snapshots"`
	}
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return fmt.Errorf("parse backup: %v", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %v", err)
	}
	defer tx.Rollback()

	clearTables := []string{
		"DELETE FROM devices",
		"DELETE FROM recordings",
		"DELETE FROM schedules",
		"DELETE FROM snapshot_configs",
	}
	for _, q := range clearTables {
		tx.Exec(q)
	}

	for _, d := range data.Devices {
		tx.Exec("INSERT INTO devices VALUES (?,?,?,?,?,?,?,?,?)",
			d.ID, d.Name, d.Protocol, d.URL, d.Username, d.Password, d.Status, d.GroupName, d.CreatedAt)
	}
	for _, r := range data.Recordings {
		tx.Exec("INSERT INTO recordings VALUES (?,?,?,?,?,?,?,?,?,?)",
			r.ID, r.DeviceID, r.FilePath, r.PlayURL, r.StartTime, r.EndTime, r.Duration, r.FileSize, r.EventType, r.CreatedAt)
	}
	for _, sc := range data.Schedules {
		en := 0
		if sc.Enabled {
			en = 1
		}
		tx.Exec("INSERT INTO schedules VALUES (?,?,?,?,?,?,?,?,?,?)",
			sc.ID, sc.Name, sc.DeviceID, sc.TimeStart, sc.TimeEnd, sc.Days, sc.DurationMin, en, sc.LastTriggered, sc.CreatedAt)
	}
	for _, sn := range data.Snapshots {
		en := 0
		if sn.Enabled {
			en = 1
		}
		tx.Exec("INSERT INTO snapshot_configs VALUES (?,?,?,?,?)",
			sn.ID, sn.DeviceID, en, sn.Interval, sn.Retention)
	}

	return tx.Commit()
}

// Stats returns key counts for the status endpoint
func (s *Store) Stats() (devices, tasks, recordings int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	s.db.QueryRow("SELECT COUNT(*) FROM devices").Scan(&devices)
	tasks = 0
	s.db.QueryRow("SELECT COUNT(*) FROM recordings").Scan(&recordings)
	return
}

// ============ Motion Config ============

func (s *Store) ListMotionConfigs() []*MotionConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, _ := s.db.Query("SELECT id, device_id, enabled, sensitivity, cooldown_sec FROM motion_configs")
	if rows == nil {
		return nil
	}
	defer rows.Close()
	var result []*MotionConfig
	for rows.Next() {
		m := &MotionConfig{}
		var en int
		rows.Scan(&m.ID, &m.DeviceID, &en, &m.Sensitivity, &m.CooldownSec)
		m.Enabled = en == 1
		result = append(result, m)
	}
	return result
}

func (s *Store) GetMotionConfig(deviceID string) *MotionConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := &MotionConfig{}
	var en int
	err := s.db.QueryRow("SELECT id, device_id, enabled, sensitivity, cooldown_sec FROM motion_configs WHERE device_id=?", deviceID).
		Scan(&m.ID, &m.DeviceID, &en, &m.Sensitivity, &m.CooldownSec)
	if err != nil {
		return nil
	}
	m.Enabled = en == 1
	return m
}

func (s *Store) SetMotionConfig(m *MotionConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	en := 0
	if m.Enabled {
		en = 1
	}
	existing := s.GetMotionConfig(m.DeviceID)
	if existing != nil {
		s.db.Exec("UPDATE motion_configs SET enabled=?, sensitivity=?, cooldown_sec=? WHERE device_id=?", en, m.Sensitivity, m.CooldownSec, m.DeviceID)
		m.ID = existing.ID
	} else {
		m.ID = uuid.NewString()[:8]
		s.db.Exec("INSERT INTO motion_configs (id, device_id, enabled, sensitivity, cooldown_sec) VALUES (?,?,?,?,?)",
			m.ID, m.DeviceID, en, m.Sensitivity, m.CooldownSec)
	}
}

// ============ Motion Events ============

func (s *Store) AddMotionEvent(e *MotionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID = uuid.NewString()[:8]
	e.CreatedAt = time.Now().Format(time.RFC3339)
	s.db.Exec("INSERT INTO motion_events (id, device_id, detected_at, snapshot_path, created_at, ai_class, ai_conf, verdict_mode) VALUES (?,?,?,?,?,?,?,?)",
		e.ID, e.DeviceID, e.DetectedAt, e.SnapshotPath, e.CreatedAt, e.AIClass, e.AIConf, e.VerdictMode)
}

func (s *Store) ListMotionEvents(deviceID string, limit int) []*MotionEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	rows, _ := s.db.Query("SELECT id, device_id, detected_at, snapshot_path, created_at, ai_class, ai_conf, verdict_mode FROM motion_events WHERE device_id=? ORDER BY detected_at DESC LIMIT ?", deviceID, limit)
	if rows == nil {
		return nil
	}
	defer rows.Close()
	var result []*MotionEvent
	for rows.Next() {
		e := &MotionEvent{}
		rows.Scan(&e.ID, &e.DeviceID, &e.DetectedAt, &e.SnapshotPath, &e.CreatedAt, &e.AIClass, &e.AIConf, &e.VerdictMode)
		result = append(result, e)
	}
	return result
}

// ListMotionEventsAll returns motion events grouped by device with time range
func (s *Store) ListMotionEventsAll(since time.Time) []*MotionEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, _ := s.db.Query("SELECT id, device_id, detected_at, snapshot_path, created_at, ai_class, ai_conf, verdict_mode FROM motion_events WHERE created_at > ? ORDER BY detected_at DESC LIMIT 200", since.Format(time.RFC3339))
	if rows == nil {
		return nil
	}
	defer rows.Close()
	var result []*MotionEvent
	for rows.Next() {
		e := &MotionEvent{}
		rows.Scan(&e.ID, &e.DeviceID, &e.DetectedAt, &e.SnapshotPath, &e.CreatedAt, &e.AIClass, &e.AIConf, &e.VerdictMode)
		result = append(result, e)
	}
	return result
}

// ============ Licenses ============

type License struct {
	Key       string `json:"key"`
	IsPro     bool   `json:"is_pro"`
	DeviceID  string `json:"device_id"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

func (s *Store) GenerateLicense(isPro bool, note string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := uuid.New().String()[:8]
	now := time.Now().Format(time.RFC3339)
	s.db.Exec("INSERT INTO licenses (key, is_pro, note, created_at) VALUES (?, ?, ?, ?)",
		key, boolToInt(isPro), note, now)
	return key
}

func (s *Store) CheckLicense(key string) *License {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow("SELECT key, is_pro, device_id, note, created_at, expires_at FROM licenses WHERE key = ?", key)
	l := &License{}
	if err := row.Scan(&l.Key, &l.IsPro, &l.DeviceID, &l.Note, &l.CreatedAt, &l.ExpiresAt); err != nil {
		return nil
	}
	if l.ExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339, l.ExpiresAt)
		if err == nil && time.Now().After(exp) {
			return nil
		}
	}
	return l
}

func (s *Store) BindLicense(key string, deviceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec("UPDATE licenses SET device_id = ? WHERE key = ? AND device_id = ''", deviceID, key)
	if err != nil {
		return false
	}
	affected, _ := res.RowsAffected()
	return affected > 0
}

func (s *Store) ListLicenses() []*License {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT key, is_pro, device_id, note, created_at, expires_at FROM licenses ORDER BY created_at DESC")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*License
	for rows.Next() {
		l := &License{}
		rows.Scan(&l.Key, &l.IsPro, &l.DeviceID, &l.Note, &l.CreatedAt, &l.ExpiresAt)
		result = append(result, l)
	}
	return result
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ============ Faces (Known Faces) ============

func (s *Store) ListFaces() []*KnownFace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id, label, face_hash, device_id, thumb_path, created_at, last_seen_at, seen_count, COALESCE(embedding, X''), emb_model FROM faces ORDER BY seen_count DESC, last_seen_at DESC")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*KnownFace
	for rows.Next() {
		f := &KnownFace{}
		rows.Scan(&f.ID, &f.Label, &f.FaceHash, &f.DeviceID, &f.ThumbPath, &f.CreatedAt, &f.LastSeenAt, &f.SeenCount, &f.Embedding, &f.EmbModel)
		result = append(result, f)
	}
	return result
}

func (s *Store) GetFace(id string) *KnownFace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f := &KnownFace{}
	err := s.db.QueryRow("SELECT id, label, face_hash, device_id, thumb_path, created_at, last_seen_at, seen_count, COALESCE(embedding, X''), emb_model FROM faces WHERE id=?", id).
		Scan(&f.ID, &f.Label, &f.FaceHash, &f.DeviceID, &f.ThumbPath, &f.CreatedAt, &f.LastSeenAt, &f.SeenCount, &f.Embedding, &f.EmbModel)
	if err != nil {
		return nil
	}
	return f
}

// FindFaceByHash finds a face by exact hash match (for dedup)
func (s *Store) FindFaceByHash(hash string) *KnownFace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f := &KnownFace{}
	err := s.db.QueryRow("SELECT id, label, face_hash, device_id, thumb_path, created_at, last_seen_at, seen_count, COALESCE(embedding, X''), emb_model FROM faces WHERE face_hash=?", hash).
		Scan(&f.ID, &f.Label, &f.FaceHash, &f.DeviceID, &f.ThumbPath, &f.CreatedAt, &f.LastSeenAt, &f.SeenCount, &f.Embedding, &f.EmbModel)
	if err != nil {
		return nil
	}
	return f
}

// AddFace inserts a new known face. Sets ID and timestamps.
func (s *Store) AddFace(f *KnownFace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.ID = uuid.NewString()[:8]
	now := time.Now().Format(time.RFC3339)
	f.CreatedAt = now
	f.LastSeenAt = now
	f.SeenCount = 1
	s.db.Exec("INSERT INTO faces (id, label, face_hash, device_id, thumb_path, created_at, last_seen_at, seen_count, embedding, emb_model) VALUES (?,?,?,?,?,?,?,?,?,?)",
		f.ID, f.Label, f.FaceHash, f.DeviceID, f.ThumbPath, f.CreatedAt, f.LastSeenAt, f.SeenCount, f.Embedding, f.EmbModel)
}

// UpdateFaceEmbedding 为已知人脸补提/更新 512d 向量(缝B)。
func (s *Store) UpdateFaceEmbedding(id string, emb []byte, model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("UPDATE faces SET embedding=?, emb_model=? WHERE id=?", emb, model, id)
}

// UpdateFaceSeen updates last_seen_at and seen_count for a known face.
func (s *Store) UpdateFaceSeen(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Format(time.RFC3339)
	s.db.Exec("UPDATE faces SET last_seen_at=?, seen_count=seen_count+1 WHERE id=?", now, id)
}

// UpdateFaceLabel updates the label/name of a known face.
func (s *Store) UpdateFaceLabel(id, label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("UPDATE faces SET label=? WHERE id=?", label, id)
}

func (s *Store) DeleteFace(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("DELETE FROM faces WHERE id=?", id)
}

// ============ Face Events ============

func (s *Store) AddFaceEvent(e *FaceEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID = uuid.NewString()[:8]
	e.CreatedAt = time.Now().Format(time.RFC3339)
	s.db.Exec("INSERT INTO face_events (id, motion_event_id, device_id, face_id, label, confidence, score, bounds, thumb_path, detected_at, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		e.ID, e.MotionEventID, e.DeviceID, e.FaceID, e.Label, e.Confidence, e.Score, e.Bounds, e.ThumbPath, e.DetectedAt, e.CreatedAt)
}

// ListFaceEvents returns face events, optionally filtered by device_id.
func (s *Store) ListFaceEvents(deviceID string, limit int) []*FaceEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if deviceID != "" {
		rows, err = s.db.Query("SELECT id, motion_event_id, device_id, face_id, label, confidence, score, bounds, thumb_path, detected_at, created_at FROM face_events WHERE device_id=? ORDER BY detected_at DESC LIMIT ?", deviceID, limit)
	} else {
		rows, err = s.db.Query("SELECT id, motion_event_id, device_id, face_id, label, confidence, score, bounds, thumb_path, detected_at, created_at FROM face_events ORDER BY detected_at DESC LIMIT ?", limit)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*FaceEvent
	for rows.Next() {
		e := &FaceEvent{}
		rows.Scan(&e.ID, &e.MotionEventID, &e.DeviceID, &e.FaceID, &e.Label, &e.Confidence, &e.Score, &e.Bounds, &e.ThumbPath, &e.DetectedAt, &e.CreatedAt)
		result = append(result, e)
	}
	return result
}

// ListFaceEventsByMotion returns all face events for a specific motion event.
func (s *Store) ListFaceEventsByMotion(motionEventID string) []*FaceEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id, motion_event_id, device_id, face_id, label, confidence, score, bounds, thumb_path, detected_at, created_at FROM face_events WHERE motion_event_id=? ORDER BY score DESC", motionEventID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*FaceEvent
	for rows.Next() {
		e := &FaceEvent{}
		rows.Scan(&e.ID, &e.MotionEventID, &e.DeviceID, &e.FaceID, &e.Label, &e.Confidence, &e.Score, &e.Bounds, &e.ThumbPath, &e.DetectedAt, &e.CreatedAt)
		result = append(result, e)
	}
	return result
}

// DeleteFaceEvent removes a face event record.
func (s *Store) DeleteFaceEvent(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("DELETE FROM face_events WHERE id=?", id)
}

// ============ Retention / Storage Management ============

type RetentionConfig struct {
	ID              string  `json:"id"`
	RetentionDays   int     `json:"retention_days"`
	MaxDiskUsagePct float64 `json:"max_disk_usage_pct"`
	Enabled         bool    `json:"enabled"`
	UpdatedAt       string  `json:"updated_at"`
}

// GetRetentionConfig returns the current retention config, inserting defaults if none exists.
func (s *Store) GetRetentionConfig() *RetentionConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	rc := &RetentionConfig{}
	err := s.db.QueryRow("SELECT id, retention_days, max_disk_usage_pct, enabled, updated_at FROM retention_config LIMIT 1").
		Scan(&rc.ID, &rc.RetentionDays, &rc.MaxDiskUsagePct, &rc.Enabled, &rc.UpdatedAt)
	if err != nil {
		// Insert default
		rc.ID = uuid.NewString()[:8]
		rc.RetentionDays = 30
		rc.MaxDiskUsagePct = 80.0
		rc.Enabled = true
		rc.UpdatedAt = time.Now().Format(time.RFC3339)
		s.db.Exec("INSERT INTO retention_config (id, retention_days, max_disk_usage_pct, enabled, updated_at) VALUES (?,?,?,?,?)",
			rc.ID, rc.RetentionDays, rc.MaxDiskUsagePct, 1, rc.UpdatedAt)
	}
	return rc
}

// UpdateRetentionConfig saves a new retention config.
func (s *Store) UpdateRetentionConfig(rc *RetentionConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rc.UpdatedAt = time.Now().Format(time.RFC3339)
	existing := &RetentionConfig{}
	err := s.db.QueryRow("SELECT id FROM retention_config LIMIT 1").Scan(&existing.ID)
	if err != nil {
		rc.ID = uuid.NewString()[:8]
		en := 0
		if rc.Enabled {
			en = 1
		}
		s.db.Exec("INSERT INTO retention_config (id, retention_days, max_disk_usage_pct, enabled, updated_at) VALUES (?,?,?,?,?)",
			rc.ID, rc.RetentionDays, rc.MaxDiskUsagePct, en, rc.UpdatedAt)
		return
	}
	en := 0
	if rc.Enabled {
		en = 1
	}
	s.db.Exec("UPDATE retention_config SET retention_days=?, max_disk_usage_pct=?, enabled=?, updated_at=? WHERE id=?",
		rc.RetentionDays, rc.MaxDiskUsagePct, en, rc.UpdatedAt, existing.ID)
}

// DeleteOldRecordings removes recordings and their files that exceed retention or disk usage.
// Returns count of deleted recordings.
func (s *Store) DeleteOldRecordings(rc *RetentionConfig) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -rc.RetentionDays).Format(time.RFC3339)
	rows, err := s.db.Query("SELECT id, file_path FROM recordings WHERE created_at < ?", cutoff)
	if err != nil {
		log.Printf("[storage] query old recordings: %v", err)
		return 0
	}
	defer rows.Close()

	var ids []string
	var paths []string
	for rows.Next() {
		var id, fp string
		rows.Scan(&id, &fp)
		ids = append(ids, id)
		paths = append(paths, fp)
	}

	if len(ids) == 0 {
		return 0
	}

	// Delete from DB
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := "DELETE FROM recordings WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	s.db.Exec(q, args...)

	// Delete files (best-effort)
	for _, fp := range paths {
		if fp != "" {
			os.Remove(fp)
		}
	}

	log.Printf("[storage] cleaned %d old recordings (retention: %d days)", len(ids), rc.RetentionDays)
	return len(ids)
}

// DeleteRecording removes a single recording from DB and filesystem.
func (s *Store) DeleteRecording(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var fp string
	s.db.QueryRow("SELECT file_path FROM recordings WHERE id=?", id).Scan(&fp)
	s.db.Exec("DELETE FROM recordings WHERE id=?", id)
	if fp != "" {
		os.Remove(fp)
	}
}

// DiskUsage returns total, used, free bytes for the data directory.
// Also returns the videos directory size (approximate via Statfs on same FS).
type DiskStats struct {
	TotalBytes     int64   `json:"total_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	FreeBytes      int64   `json:"free_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	RecordingCount int     `json:"recording_count"`
	RecordingBytes int64   `json:"recording_bytes"`
}

func (s *Store) GetDiskStats() *DiskStats {
	ds := &DiskStats{}

	// Statfs on data directory
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.dataDir, &stat); err == nil {
		ds.TotalBytes = int64(stat.Blocks) * stat.Bsize
		ds.FreeBytes = int64(stat.Bfree) * stat.Bsize
		ds.UsedBytes = ds.TotalBytes - ds.FreeBytes
		if ds.TotalBytes > 0 {
			ds.UsedPercent = float64(ds.UsedBytes) / float64(ds.TotalBytes) * 100.0
		}
	}

	// Count recordings and sum file sizes
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT COUNT(*), IFNULL(SUM(file_size),0) FROM recordings")
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			rows.Scan(&ds.RecordingCount, &ds.RecordingBytes)
		}
	}
	return ds
}
