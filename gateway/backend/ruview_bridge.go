package main

// RuView WiFi CSI bridge — exposes RuView sensing-server through the Gateway
// so the NovaSense UI can show real ESP32 CSI data natively (no iframe).
//
//   GET /api/ruview/status  → proxy of RuView /api/v1/status (source/trust)
//   WS  /api/ruview/ws      → proxy of RuView /ws/sensing (live sensing frames)
//
// The Gateway container runs with network_mode: host, so 127.0.0.1:3000/3001
// reach the co-located RuView container published ports.

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	ruviewHTTPBase = "http://127.0.0.1:3000"
	ruviewWSBase   = "ws://127.0.0.1:3001"
)

// handleRuviewStatus proxies RuView /api/v1/status (node state + trust labels).
func (s *Server) handleRuviewStatus(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(ruviewHTTPBase + "/api/v1/status")
	if err != nil {
		s.error(w, "ruview unreachable: "+err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.error(w, "ruview read failed", 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// handleRuviewWS bridges a browser WebSocket to RuView /ws/sensing.
// RuView gates the socket behind a single-use ticket (ADR-272): we mint one
// server-side, then tunnel frames both ways.
func (s *Server) handleRuviewWS(w http.ResponseWriter, r *http.Request) {
	// 1. Mint a RuView WS ticket.
	ticket, err := mintRuviewTicket()
	if err != nil {
		s.error(w, "ruview ticket failed: "+err.Error(), 502)
		return
	}

	// 2. Dial RuView sensing WS.
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	upstream, _, err := dialer.Dial(ruviewWSBase+"/ws/sensing?ticket="+ticket, nil)
	if err != nil {
		s.error(w, "ruview ws dial failed: "+err.Error(), 502)
		return
	}
	defer upstream.Close()

	// 3. Upgrade the browser connection.
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ruview-ws] upgrade: %v", err)
		return
	}
	defer conn.Close()

	// 4. Bidirectional tunnel.
	done := make(chan struct{}, 2)
	go func() { // RuView → browser
		defer func() { done <- struct{}{} }()
		for {
			mt, data, err := upstream.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}()
	go func() { // browser → RuView (mostly ping/keepalive)
		defer func() { done <- struct{}{} }()
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := upstream.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}()
	<-done
	<-done
}

func mintRuviewTicket() (string, error) {
	req, err := http.NewRequest("POST", ruviewHTTPBase+"/api/v1/ws-ticket", bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Ticket == "" {
		return "", io.ErrUnexpectedEOF
	}
	return out.Ticket, nil
}
