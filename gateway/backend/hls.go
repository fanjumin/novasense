package main

import (
	"io"
	"net/http"
	"strings"
)

// registerHLSProxy registers the HLS reverse proxy route
// Forwards /hls/* requests to MediaMTX at :8888
func (s *Server) registerHLSProxy() {
	s.mux.Handle("/hls/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := "http://127.0.0.1:8888" + strings.TrimPrefix(r.URL.Path, "/hls")
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}

		req, _ := http.NewRequest(r.Method, target, nil)
		req.Host = "127.0.0.1:8888"
		for k, v := range r.Header {
			for _, hv := range v {
				if k == "Cookie" || k == "Set-Cookie" {
					continue
				}
				req.Header.Add(k, hv)
			}
		}

		resp, err := s.hlsClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			for _, hv := range v {
				if k == "Set-Cookie" {
					hv = strings.ReplaceAll(hv, "; Secure", "")
					hv = strings.ReplaceAll(hv, "SameSite=None", "SameSite=Lax")
				}
				w.Header().Add(k, hv)
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
}

