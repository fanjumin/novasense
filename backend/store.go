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
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// ============ Data Models ============

type Device struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Status    string `json:"status"`
	GroupName string `json:"group_name"`
	CreatedAt string `json:"created_at"`
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
	ID          string `json:"id"`
	DeviceID    string `json:"device_id"`
	DetectedAt  string `json:"detected_at"`
	SnapshotPath string `json:"snapshot_path"`
	CreatedAt   string `json:"created_at"`
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
	}

	for _, ddl := range tables {
		if _, err := s.db.Exec(ddl); err != nil {
			log.Fatalf("[store] migrate failed: %v", err)
		}
	}
	log.Println("[store] database migrated")
}

// ============ Devices ============

func (s *Store) ListDevices() []*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id, name, protocol, url, username, password, status, group_name, created_at FROM devices ORDER BY created_at DESC")
	if err != nil {
		log.Printf("[store] list devices: %v", err)
		return nil
	}
	defer rows.Close()
	var result []*Device
	for rows.Next() {
		d := &Device{}
		rows.Scan(&d.ID, &d.Name, &d.Protocol, &d.URL, &d.Username, &d.Password, &d.Status, &d.GroupName, &d.CreatedAt)
		result = append(result, d)
	}
	return result
}

func (s *Store) GetDevice(id string) *Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d := &Device{}
	err := s.db.QueryRow("SELECT id, name, protocol, url, username, password, status, group_name, created_at FROM devices WHERE id=?", id).
		Scan(&d.ID, &d.Name, &d.Protocol, &d.URL, &d.Username, &d.Password, &d.Status, &d.GroupName, &d.CreatedAt)
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
		return nil
	}
	defer rows.Close()
	var result []*Recording
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
		return nil
	}
	defer rows.Close()
	var result []*Schedule
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
		return nil
	}
	defer rows.Close()
	var result []*SnapshotConfig
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
		Devices    []*Device        `json:"devices"`
		Recordings []*Recording     `json:"recordings"`
		Schedules  []*Schedule      `json:"schedules"`
		Snapshots  []*SnapshotConfig `json:"snapshots"`
		ExportedAt string           `json:"exported_at"`
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
		Devices    []*Device        `json:"devices"`
		Recordings []*Recording     `json:"recordings"`
		Schedules  []*Schedule      `json:"schedules"`
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
	s.db.Exec("INSERT INTO motion_events (id, device_id, detected_at, snapshot_path, created_at) VALUES (?,?,?,?,?)",
		e.ID, e.DeviceID, e.DetectedAt, e.SnapshotPath, e.CreatedAt)
}

func (s *Store) ListMotionEvents(deviceID string, limit int) []*MotionEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	rows, _ := s.db.Query("SELECT id, device_id, detected_at, snapshot_path, created_at FROM motion_events WHERE device_id=? ORDER BY detected_at DESC LIMIT ?", deviceID, limit)
	if rows == nil {
		return nil
	}
	defer rows.Close()
	var result []*MotionEvent
	for rows.Next() {
		e := &MotionEvent{}
		rows.Scan(&e.ID, &e.DeviceID, &e.DetectedAt, &e.SnapshotPath, &e.CreatedAt)
		result = append(result, e)
	}
	return result
}

// ListMotionEventsAll returns motion events grouped by device with time range
func (s *Store) ListMotionEventsAll(since time.Time) []*MotionEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, _ := s.db.Query("SELECT id, device_id, detected_at, snapshot_path, created_at FROM motion_events WHERE created_at > ? ORDER BY detected_at DESC LIMIT 200", since.Format(time.RFC3339))
	if rows == nil {
		return nil
	}
	defer rows.Close()
	var result []*MotionEvent
	for rows.Next() {
		e := &MotionEvent{}
		rows.Scan(&e.ID, &e.DeviceID, &e.DetectedAt, &e.SnapshotPath, &e.CreatedAt)
		result = append(result, e)
	}
	return result
}
