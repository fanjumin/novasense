package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// CameraSetting represents a single adjustable image parameter
type CameraSetting struct {
	Name  string  `json:"name"`
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Step  float64 `json:"step"`
}

// handleCameraSettings reads/writes camera image parameters via ISAPI/CGI
// GET /api/camera/{id}/settings — returns current values
// PUT /api/camera/{id}/settings — updates values
func (s *Server) handleCameraSettings(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/camera/")
	path = strings.TrimSuffix(path, "/settings")
	deviceID := strings.TrimRight(path, "/")

	device := s.store.GetDevice(deviceID)
	if device == nil {
		s.error(w, "device not found", 404)
		return
	}

	baseURL := buildCameraHTTPURL(device.URL)
	if baseURL == "" || device.Protocol != "rtsp" {
		s.error(w, "camera settings not supported for this device type", 400)
		return
	}

	switch r.Method {
	case "GET":
		settings := probeCameraSettings(baseURL, device.Username, device.Password)
		// Try SDK via port 34567 first for RTSP cameras
		ip := extractIP(device.URL)
		sdkUser := device.Username
		if sdkUser == "" {
			sdkUser = "admin"
		}
		if dev, err := LoginDev(ip, sdkUser, device.Password, 34567); err == nil {
			if color, err := dev.GetColor(0); err == nil {
				settings = []CameraSetting{
					{Name: "brightness", Label: "亮度", Value: float64(color.Brightness), Min: 0, Max: 100, Step: 1},
					{Name: "contrast", Label: "对比度", Value: float64(color.Contrast), Min: 0, Max: 100, Step: 1},
					{Name: "saturation", Label: "饱和度", Value: float64(color.Saturation), Min: 0, Max: 100, Step: 1},
					{Name: "sharpness", Label: "锐度", Value: float64(color.Sharpness), Min: 0, Max: 15, Step: 1},
				}
				log.Printf("[camera-settings] got values via SDK from %s", ip)
			} else {
				log.Printf("[camera-settings] SDK GetColor failed: %v", err)
			}
			LogoutDev(dev)
		} else {
			log.Printf("[camera-settings] SDK login to %s failed: %v", ip, err)
		}
		s.json(w, settings)
	case "PUT":
		var req struct {
			Name  string  `json:"name"`
			Value float64 `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.error(w, "invalid JSON", 400)
			return
		}
		// Try SDK first
		ip := extractIP(device.URL)
		sdkUser := device.Username
		if sdkUser == "" {
			sdkUser = "admin"
		}
		if dev, err := LoginDev(ip, sdkUser, device.Password, 34567); err == nil {
			if color, err := dev.GetColor(0); err == nil {
				switch req.Name {
				case "brightness":
					color.Brightness = int(req.Value)
				case "contrast":
					color.Contrast = int(req.Value)
				case "saturation":
					color.Saturation = int(req.Value)
				case "sharpness":
					color.Sharpness = int(req.Value)
				}
				dev.SetColor(0, color)
			}
			LogoutDev(dev)
		}
		// Also try HTTP CGI as fallback
		_ = setCameraSetting(baseURL, device.Username, device.Password, req.Name, req.Value)
		s.json(w, map[string]interface{}{"status": "ok"})
	default:
		s.error(w, "method not allowed", 405)
	}
}

func buildCameraHTTPURL(rtspURL string) string {
	baseURL := strings.TrimSuffix(rtspURL, "/ch1/main/av_stream")
	baseURL = strings.TrimSuffix(baseURL, "/ch1/sub/av_stream")
	baseURL = strings.TrimSuffix(baseURL, "/stream1")
	baseURL = strings.TrimSuffix(baseURL, "/Streaming/Channels/101")
	baseURL = strings.TrimSuffix(baseURL, ":554")
	if strings.HasPrefix(baseURL, "rtsp://") {
		baseURL = "http://" + strings.TrimPrefix(baseURL, "rtsp://")
	}
	return baseURL
}

func extractIP(deviceURL string) string {
	u := deviceURL
	for _, prefix := range []string{"rtsp://", "http://", "https://"} {
		u = strings.TrimPrefix(u, prefix)
	}
	// Remove user:pass@ prefix if present
	if idx := strings.Index(u, "@"); idx >= 0 {
		u = u[idx+1:]
	}
	if idx := strings.Index(u, ":"); idx >= 0 {
		u = u[:idx]
	}
	if idx := strings.Index(u, "/"); idx >= 0 {
		u = u[:idx]
	}
	return u
}

func probeCameraSettings(baseURL, username, password string) []CameraSetting {
	defaultSettings := []CameraSetting{
		{Name: "brightness", Label: "亮度", Value: 128, Min: 0, Max: 255, Step: 1},
		{Name: "contrast", Label: "对比度", Value: 128, Min: 0, Max: 255, Step: 1},
		{Name: "saturation", Label: "饱和度", Value: 128, Min: 0, Max: 255, Step: 1},
		{Name: "sharpness", Label: "锐度", Value: 128, Min: 0, Max: 255, Step: 1},
	}

	var values map[string]float64

	// Try ONVIF first (needs SOAP POST)
	if body, err := onvifGetImaging(baseURL, username, password); err == nil {
		if val, ok := parseONVIFImaging(body); ok {
			values = val
			log.Printf("[camera-settings] got values from ONVIF")
		} else {
			log.Printf("[camera-settings] ONVIF response parsed but no imaging values found")
		}
	} else {
		log.Printf("[camera-settings] ONVIF probe error: %v", err)
	}

	// Fallback to CGI endpoints (GET/POST-based)
	if values == nil {
		cgiEndpoints := []struct {
			url    string
			parser func(string) (map[string]float64, bool)
			post   string // POST body (empty = GET request)
		}{
			{fmt.Sprintf("%s/cgi-bin/getconfig.cgi", baseURL), parseJsonConfig, `{"cmd":"GetImage","param":{}}`},
			{fmt.Sprintf("%s/cgi-bin/getconfig.cgi", baseURL), parseJsonConfig, `{"cmd":"GetAbility","param":{}}`},
			{fmt.Sprintf("%s/cgi-bin/param.cgi?cmd=getimageattr", baseURL), parseXmImageAttr, ""},
			{fmt.Sprintf("%s/cgi-bin/Config.cgi?action=getconfig&name=Image", baseURL), parseXmConfig, ""},
			{fmt.Sprintf("%s/cgi-bin/setting.cgi?cmd=getimageattr", baseURL), parseXmImageAttr, ""},
		}
		for _, ep := range cgiEndpoints {
			var body string
			var err error
			if ep.post != "" {
				body, err = httpPostJSON(ep.url, ep.post, username, password, 3)
			} else {
				body, err = httpGetWithAuth(ep.url, username, password, 3)
			}
			if err != nil {
				log.Printf("[camera-settings] probe error %s: %v", ep.url, err)
				continue
			}
			if val, ok := ep.parser(body); ok {
				values = val
				log.Printf("[camera-settings] got values from %s", ep.url)
				break
			}
		}
	}

	result := make([]CameraSetting, len(defaultSettings))
	for i, s := range defaultSettings {
		result[i] = s
		if values != nil {
			if v, ok := values[s.Name]; ok {
				result[i].Value = v
			}
		}
	}
	return result
}

func httpGetWithAuth(url, username, password string, timeoutSec int) (string, error) {
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}

	// Step 1: Try with Digest auth first (XiongMai cameras need it)
	if username != "" {
		body, err := digestAuthGet(client, url, username, password)
		if err == nil {
			return body, nil
		}
		// Fall through to try Basic
	}

	// Step 2: Fallback to Basic auth
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

// digestAuthGet performs HTTP GET with Digest authentication
func digestAuthGet(client *http.Client, url, username, password string) (string, error) {
	// Step 1: Make initial request to get WWW-Authenticate
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()

	if resp.StatusCode != 401 {
		// No auth required or already authed
		// Retry without auth
		req2, _ := http.NewRequest("GET", url, nil)
		resp2, err := client.Do(req2)
		if err != nil {
			return "", err
		}
		defer resp2.Body.Close()
		body, _ := io.ReadAll(resp2.Body)
		return string(body), nil
	}

	// Step 2: Parse WWW-Authenticate header
	authHeader := resp.Header.Get("WWW-Authenticate")
	if authHeader == "" {
		return "", fmt.Errorf("no WWW-Authenticate header")
	}

	params := parseDigestParams(authHeader)
	realm := params["realm"]
	nonce := params["nonce"]
	qop := params["qop"]
	opaque := params["opaque"]

	// Step 3: Calculate digest response
	ha1 := md5Hash(username + ":" + realm + ":" + password)
	ha2 := md5Hash("GET:" + url)
	nc := "00000001"
	cnonce := fmt.Sprintf("%x", time.Now().UnixNano())
	response := md5Hash(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":auth:" + ha2)

	// Step 4: Re-send with Authorization header
	auth := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", qop=%s, nc=%s, cnonce="%s", response="%s", opaque="%s"`,
		username, realm, nonce, url, qop, nc, cnonce, response, opaque,
	)
	req2, _ := http.NewRequest("GET", url, nil)
	req2.Header.Set("Authorization", auth)
	resp2, err := client.Do(req2)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)
	return string(body), nil
}

func md5Hash(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}

func parseDigestParams(header string) map[string]string {
	params := make(map[string]string)
	// Remove "Digest " prefix
	header = strings.TrimPrefix(header, "Digest ")
	header = strings.TrimPrefix(header, "digest ")
	// Split by comma
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if eq := strings.Index(part, "="); eq >= 0 {
			key := strings.TrimSpace(part[:eq])
			val := strings.TrimSpace(part[eq+1:])
			val = strings.Trim(val, "\"")
			params[key] = val
		}
	}
	return params
}

func parseXmImageAttr(body string) (map[string]float64, bool) {
	vals := make(map[string]float64)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		name := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		var fv float64
		if _, err := fmt.Sscanf(val, "%f", &fv); err == nil {
			vals[name] = fv
		} else {
			var iv int
			if _, err := fmt.Sscanf(val, "%d", &iv); err == nil {
				vals[name] = float64(iv)
			}
		}
	}
	return vals, len(vals) > 0
}

func parseXmConfig(body string) (map[string]float64, bool) {
	vals := make(map[string]float64)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		name := mapXmKey(key)
		if name != "" {
			var fv float64
			if _, err := fmt.Sscanf(val, "%f", &fv); err == nil {
				vals[name] = fv
			} else {
				var iv int
				if _, err := fmt.Sscanf(val, "%d", &iv); err == nil {
					vals[name] = float64(iv)
				}
			}
		}
	}
	return vals, len(vals) > 0
}

func mapXmKey(key string) string {
	lower := strings.ToLower(key)
	if strings.Contains(lower, "brightness") || strings.Contains(lower, "bright") {
		return "brightness"
	}
	if strings.Contains(lower, "contrast") {
		return "contrast"
	}
	if strings.Contains(lower, "saturation") {
		return "saturation"
	}
	if strings.Contains(lower, "sharpness") {
		return "sharpness"
	}
	return ""
}

// parseJsonConfig parses JSON CGI response for image settings
// Format: {"cmd":"GetImage","param":{"Brightness":128,"Contrast":128,...}}
func parseJsonConfig(body string) (map[string]float64, bool) {
	var resp struct {
		Cmd   string                 `json:"cmd"`
		Param map[string]interface{} `json:"param"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, false
	}
	vals := make(map[string]float64)
	imageKeys := map[string]string{
		"Brightness": "brightness",
		"Contrast":   "contrast",
		"Saturation": "saturation",
		"Sharpness":  "sharpness",
	}
	for k, v := range resp.Param {
		if target, ok := imageKeys[k]; ok {
			if f, ok := toFloat64(v); ok {
				vals[target] = f
			}
		}
	}
	return vals, len(vals) > 0
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(val, "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// httpPostJSON sends POST with JSON body for CGI calls
func httpPostJSON(url, jsonBody, username, password string, timeoutSec int) (string, error) {
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	req, err := http.NewRequest("POST", url, strings.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if username != "" {
		req.SetBasicAuth(username, password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

// parseONVIFImaging tries to get imaging settings via ONVIF SOAP
func parseONVIFImaging(body string) (map[string]float64, bool) {
	// body is the response from ONVIF device_service
	// We need to parse the SOAP XML for imaging values
	// The response is stored in the body parameter but this function
	// is called from the probe loop with the HTTP response body.
	// Since ONVIF response is SOAP XML, we need to search for specific elements.
	if body == "" {
		return nil, false
	}
	vals := make(map[string]float64)
	// Look for brightness in the response
	if idx := strings.Index(strings.ToLower(body), "brightness"); idx >= 0 {
		// Found ONVIF response - try to extract values
		// ONVIF typical format: <Brightness>50</Brightness>
		if v := extractONVIFValue(body, "Brightness"); v >= 0 {
			vals["brightness"] = v
		}
		if v := extractONVIFValue(body, "Contrast"); v >= 0 {
			vals["contrast"] = v
		}
		if v := extractONVIFValue(body, "Saturation"); v >= 0 {
			vals["saturation"] = v
		}
		if v := extractONVIFValue(body, "Sharpness"); v >= 0 {
			vals["sharpness"] = v
		}
		return vals, len(vals) > 0
	}
	return nil, false
}

func extractONVIFValue(xml, tag string) float64 {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(xml, open)
	if start < 0 {
		return -1
	}
	start += len(open)
	end := strings.Index(xml[start:], close)
	if end < 0 {
		return -1
	}
	var v float64
	if _, err := fmt.Sscanf(xml[start:start+end], "%f", &v); err == nil {
		return v
	}
	return -1
}

// onvifGetImaging sends ONVIF GetProfiles → GetVideoSource → GetImagingSettings
func onvifGetImaging(baseURL, username, password string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	onvifURL := baseURL + "/onvif/device_service"

	// Step 1: GetProfiles to find video source token
	profilesReq := `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
  xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
  <soap:Body><trt:GetProfiles/></soap:Body>
</soap:Envelope>`

	body, err := onvifSOAPCall(client, onvifURL, profilesReq, username, password)
	if err != nil {
		return "", err
	}

	// Extract video source token from profile
	token := extractONVIFToken(body, "videoSourceToken")
	if token == "" {
		// Try with namespace prefix
		token = extractONVIFToken(body, "VideoSourceToken")
	}
	if token == "" {
		return body, nil // return what we got even without token
	}

	// Step 2: Get imaging settings for this source
	imgReq := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
  xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
  xmlns:img="http://www.onvif.org/ver20/imaging/wsdl">
  <soap:Body><img:GetImagingSettings>
    <VideoSourceToken>%s</VideoSourceToken>
  </img:GetImagingSettings></soap:Body>
</soap:Envelope>`, token)

	body2, err := onvifSOAPCall(client, onvifURL, imgReq, username, password)
	if err != nil {
		return body, nil // return profiles body even if imaging fails
	}
	return body2, nil
}

func onvifSOAPCall(client *http.Client, url, soapBody, username, password string) (string, error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(soapBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/soap+xml;charset=utf-8")
	if username != "" {
		req.SetBasicAuth(username, password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

func extractONVIFToken(xml, attr string) string {
	// Try: attr="tokenValue"
	search := attr + `="`
	start := strings.Index(xml, search)
	if start >= 0 {
		start += len(search)
		end := strings.Index(xml[start:], `"`)
		if end > 0 {
			return xml[start : start+end]
		}
	}
	return ""
}

// setCameraSetting sends a command to set a camera parameter
func setCameraSetting(baseURL, username, password, name string, value float64) string {
	urls := []string{
		fmt.Sprintf("%s/cgi-bin/param.cgi?cmd=setimageattr&%s=%.0f", baseURL, name, value),
		fmt.Sprintf("%s/cgi-bin/setting.cgi?cmd=setimageattr&%s=%.0f", baseURL, name, value),
		fmt.Sprintf("%s/cgi-bin/Config.cgi?action=setconfig&name=Image&%s=%.0f", baseURL, name, value),
	}

	for _, url := range urls {
		body, err := httpGetWithAuth(url, username, password, 3)
		if err != nil {
			continue
		}
		return body
	}
	return ""
}
