package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ============ Data Models ============

type DiscoveredDevice struct {
	IP         string   `json:"ip"`
	MAC        string   `json:"mac,omitempty"`
	MACVendor  string   `json:"mac_vendor,omitempty"`
	Ports      []int    `json:"ports"`
	Brand      string   `json:"brand,omitempty"`
	Model      string   `json:"model,omitempty"`
	BrandCN    string   `json:"brand_cn,omitempty"`
	Protocol   string   `json:"protocol"`
	RTSPURLs   []string `json:"rtsp_urls,omitempty"`
	HTTPURL    string   `json:"http_url,omitempty"`
	HWAddr     string   `json:"hwaddr,omitempty"`
	Confidence int      `json:"confidence"`
	Method     string   `json:"method"`
	DeviceType string   `json:"device_type,omitempty"`
}

type DiscoveryProgress struct {
	Scanning bool                          `json:"scanning"`
	Progress string                        `json:"progress"`
	Results  map[string]*DiscoveredDevice `json:"results"`
	Error    string                        `json:"error,omitempty"`
}

// ============ Discovery Manager ============

type DiscoveryManager struct {
	mu       sync.Mutex
	scanning bool
	progress string
	results  map[string]*DiscoveredDevice
	stopCh   chan struct{}
}

func NewDiscoveryManager() *DiscoveryManager {
	return &DiscoveryManager{
		results: make(map[string]*DiscoveredDevice),
	}
}

// ============ MAC OUI Database ============

type OUIEntry struct {
	Brand      string
	BrandCN    string
	DeviceType string // camera, nvr, ac, appliance, router, display, nas, smart-home, printer, network, unknown
}

// Device type priority by brand (used when a brand makes multiple product categories)
var brandDeviceTypes = map[string]string{
	// Cameras
	"Hikvision": "camera", "Dahua": "camera", "Uniview": "camera",
	"Xiongmai": "camera", "EZVIZ": "camera", "Axis": "camera",
	"Bosch": "camera",
	// AC
	"Gree": "ac", "Daikin": "ac", "Fujitsu": "ac",
	// Appliances
	"Siemens": "appliance",
	// Routers
	"TP-Link": "router", "Tenda": "router", "Ruijie": "router",
	"Juniper": "router", "ZTE": "router",
	// NAS
	"Synology": "nas", "QNAP": "nas", "ASUSTOR": "nas",
	"WesternDigital": "nas",
	// Printers
	"HP": "printer", "EPSON": "printer", "Canon": "printer",
	"Brother": "printer",
}

// Brands that make MULTIPLE types of products
// For these, device type is determined by port/protocol fingerprinting, not just MAC
var multiCategoryBrands = map[string]bool{
	"Xiaomi": true, "Midea": true, "Haier": true, "Huawei": true,
	"Panasonic": true, "Sony": true, "Samsung": true, "TCL": true,
	"Hisense": true, "Mitsubishi": true,
}

var macOUI = map[string]OUIEntry{
	// ======== 摄像头品牌（Camera） ========
	"00:15:5D": {"Hikvision", "海康威视", "camera"},
	"44:19:B6": {"Hikvision", "海康威视", "camera"},
	"8C:EA:1B": {"Hikvision", "海康威视", "camera"},
	"B4:A9:5A": {"Dahua", "大华", "camera"},
	"5C:76:19": {"Dahua", "大华", "camera"},
	"5C:C6:8B": {"Dahua", "大华", "camera"},
	"00:24:1C": {"Uniview", "宇视", "camera"},
	"50:CD:32": {"Xiongmai", "雄迈", "camera"},
	"14:CF:92": {"EZVIZ", "萤石", "camera"},
	"2C:26:5F": {"EZVIZ", "萤石", "camera"},
	"BC:32:5F": {"EZVIZ", "萤石", "camera"},
	"00:0B:AB": {"Axis", "Axis", "camera"},
	"00:40:8C": {"Bosch", "Bosch", "camera"},
	"00:12:6D": {"DSE", "大华(DSE)", "camera"},
	"00:E0:1C": {"etc", "中国普天", "camera"},

	// ======== 空调品牌（Air Conditioner） ========
	"08:11:96": {"Gree", "格力", "ac"},
	"34:65:B3": {"Gree", "格力", "ac"},
	"54:27:58": {"Gree", "格力", "ac"},
	"F4:5C:89": {"Daikin", "大金", "ac"},
	"00:04:63": {"Fujitsu", "富士通", "ac"},

	// ======== 路由器/网络设备 ========
	"F4:3E:61": {"TP-Link", "TP-Link", "router"},
	"50:C7:BF": {"TP-Link", "TP-Link", "router"},
	"CC:B2:55": {"TP-Link", "TP-Link", "router"},
	"E0:63:DA": {"TP-Link", "TP-Link", "router"},
	"D4:EE:07": {"TP-Link", "TP-Link", "router"},
	"00:25:9D": {"Juniper", "Juniper", "router"},
	"00:1A:A0": {"ZTE", "中兴", "router"},
	"68:72:51": {"ZTE", "中兴", "router"},
	"94:D9:B3": {"Tenda", "腾达", "router"},
	"00:B0:0C": {"Tenda", "腾达", "router"},
	"AC:84:C6": {"Tenda", "腾达", "router"},
	"20:1B:2C": {"Uniscom", "烽火", "router"},
	"AC:11:24": {"Ruijie", "锐捷", "router"},

	// ======== NAS 存储 ========
	"00:11:32": {"Synology", "群晖", "nas"},
	"90:02:A9": {"WesternDigital", "西部数据", "nas"},
	"0C:9D:92": {"QNAP", "威联通", "nas"},
	"00:08:9B": {"QNAP", "威联通", "nas"},
	"A0:21:B7": {"ASUSTOR", "华芸", "nas"},

	// ======== 打印机 ========
	"00:1E:37": {"HP", "惠普", "printer"},
	"00:21:5A": {"HP", "惠普", "printer"},
	"00:03:6B": {"EPSON", "爱普生", "printer"},
	"00:00:74": {"EPSON", "爱普生", "printer"},
	"00:1E:58": {"Canon", "佳能", "printer"},
	"00:25:36": {"Canon", "佳能", "printer"},
	"00:22:68": {"Brother", "兄弟", "printer"},
	"00:80:77": {"Brother", "兄弟", "printer"},

	// ======== 多品类品牌（type detected by port fingerprint） ========
	// 小米: 路由器/摄像头/智能家居/电视
	"8C:AE:4C": {"Xiaomi", "小米", "multi"},
	"48:8F:5A": {"Xiaomi", "小米", "multi"},
	"F0:B4:29": {"Xiaomi", "小米", "multi"},
	"C0:4A:00": {"Xiaomi", "小米", "multi"},
	// 华为: 路由器/智能家居
	"3C:08:F6": {"Huawei", "华为", "multi"},
	"18:31:BF": {"Huawei", "华为", "multi"},
	"30:B4:9E": {"Huawei", "华为", "multi"},
	// 美的: 空调/冰箱/洗衣机/智能家居
	"00:15:6D": {"Midea", "美的", "multi"},
	"30:5E:3B": {"Midea", "美的", "multi"},
	"5C:DA:D4": {"Midea", "美的", "multi"},
	// 海尔: 空调/冰箱/洗衣机
	"80:1A:1A": {"Haier", "海尔", "multi"},
	// 海信: 空调/电视
	"D0:62:65": {"Hisense", "海信", "multi"},
	// TCL: 空调/电视
	"98:FE:94": {"TCL", "TCL", "multi"},
	"B0:75:D5": {"TCL", "TCL", "multi"},
	// 松下: 摄像头/电视/空调
	"00:1C:DF": {"Panasonic", "松下", "multi"},
	// 索尼: 摄像头/电视
	"00:60:6E": {"Sony", "索尼", "multi"},
	// 三星: 摄像头/电视
	"00:02:D1": {"Samsung", "三星", "multi"},
	// 三菱: 空调/冰箱
	"E0:3F:33": {"Mitsubishi", "三菱电机", "multi"},
	"AC:51:11": {"Mitsubishi", "三菱", "multi"},
	// 西门子: 冰箱/洗衣机
	"A8:23:30": {"Siemens", "西门子", "multi"},
}

// ============ RTSP Path Fingerprints ============

type CameraFingerprint struct {
	Brand     string
	BrandCN   string
	RTSPPaths []string
	HTTPCheck string
	HTTPTitle string
	Ports     []int
}

var cameraFingerprints = []CameraFingerprint{
	{
		Brand: "Hikvision", BrandCN: "海康威视",
		RTSPPaths: []string{"/Streaming/Channels/101", "/ch1/main/av_stream", "/h264/ch1/main/av_stream", "/H264/ch1/main/av_stream", "/live", "/1"},
		HTTPCheck: "Hikvision",
		HTTPTitle: "Hikvision",
		Ports:     []int{80, 554, 443, 8000},
	},
	{
		Brand: "Dahua", BrandCN: "大华",
		RTSPPaths: []string{"/cam/realmonitor?channel=1&subtype=0", "/live/ch0", "/live1/ch0", "/h264/ch0"},
		HTTPCheck: "Dahua",
		HTTPTitle: "Dahua",
		Ports:     []int{80, 554, 37777, 8080},
	},
	{
		Brand: "TP-Link", BrandCN: "TP-Link",
		RTSPPaths: []string{"/live/ch0", "/stream1", "/video1/h264", "/h264", "/1"},
		HTTPCheck: "TP-LINK",
		HTTPTitle: "TP-LINK",
		Ports:     []int{80, 554, 443},
	},
	{
		Brand: "EZVIZ", BrandCN: "萤石",
		RTSPPaths: []string{"/ch1/main/av_stream", "/live/ch0", "/h264_stream"},
		HTTPCheck: "EZVIZ",
		HTTPTitle: "",
		Ports:     []int{80, 554, 443},
	},
	{
		Brand: "Uniview", BrandCN: "宇视",
		RTSPPaths: []string{"/media/video1", "/ch1/main/av_stream", "/live/ch0"},
		HTTPCheck: "Uniview",
		HTTPTitle: "Uniview",
		Ports:     []int{80, 554},
	},
	{
		Brand: "Xiongmai", BrandCN: "雄迈",
		RTSPPaths: []string{"/live/ch0", "/h264/ch0", "/mjpeg/ch0"},
		HTTPCheck: "XiongMai",
		HTTPTitle: "",
		Ports:     []int{80, 554, 34567},
	},
	{
		Brand: "Xiaomi", BrandCN: "小米",
		RTSPPaths: []string{"/live/ch0", "/h264_stream", "/stream", "/video"},
		HTTPCheck: "Xiaomi",
		HTTPTitle: "Xiaomi",
		Ports:     []int{80, 554, 443},
	},
	{
		Brand: "Huawei", BrandCN: "华为",
		RTSPPaths: []string{"/live/ch0", "/h264/ch0", "/streaming/channels/1"},
		HTTPCheck: "Huawei",
		HTTPTitle: "Huawei",
		Ports:     []int{80, 554},
	},
	{
		Brand: "Generic", BrandCN: "通用",
		RTSPPaths: []string{"/live", "/video", "/h264", "/live/ch0", "/ch1/main/av_stream", "/", "/stream", "/cam1", "/video1", "/h264_stream", "/mjpeg", "/rtsp"},
		HTTPCheck: "",
		HTTPTitle: "",
		Ports:     []int{554, 80, 8080, 8554},
	},
}

// ============ ARP Table Scan ============

func getARPTable() map[string]string {
	result := make(map[string]string)
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return result
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			ip := fields[0]
			mac := fields[3]
			if strings.Count(mac, ":") == 5 && mac != "00:00:00:00:00:00" {
				result[ip] = strings.ToUpper(mac)
			}
		}
	}
	return result
}

// ============ MAC Vendor Lookup ============

func lookupMACVendor(mac string) (string, string, string) {
	mac = strings.ToUpper(mac)
	mac = strings.ReplaceAll(mac, "-", "")
	mac = strings.ReplaceAll(mac, ":", "")
	if len(mac) < 6 {
		return "", "", ""
	}
	oui := mac[:2] + ":" + mac[2:4] + ":" + mac[4:6]
	if v, ok := macOUI[oui]; ok {
		return v.Brand, v.BrandCN, v.DeviceType
	}
	return "", "", ""
}

// ============ SSDP/UPnP Scanner ============

const ssdpAddr = "239.255.255.250:1900"

var ssdpDiscoverMsg = []byte(
	"M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 3\r\n" +
		"ST: ssdp:all\r\n" +
		"USER-AGENT: VideoStreamManager/1.0\r\n" +
		"\r\n",
)

type ssdpResponse struct {
	Location string
	Server   string
	ST       string
	USN      string
}

func parseSSDPResponse(data []byte) *ssdpResponse {
	resp := &ssdpResponse{}
	lines := strings.Split(string(data), "\r\n")
	for _, line := range lines {
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "LOCATION:") {
			resp.Location = strings.TrimSpace(line[9:])
		} else if strings.HasPrefix(upper, "SERVER:") {
			resp.Server = strings.TrimSpace(line[7:])
		} else if strings.HasPrefix(line, "ST:") {
			resp.ST = strings.TrimSpace(line[3:])
		} else if strings.HasPrefix(line, "USN:") {
			resp.USN = strings.TrimSpace(line[4:])
		}
	}
	if resp.Location == "" {
		return nil
	}
	return resp
}

type upnpDeviceDesc struct {
	XMLName xml.Name `xml:"root"`
	Device  struct {
		DeviceType   string `xml:"deviceType"`
		FriendlyName string `xml:"friendlyName"`
		Manufacturer string `xml:"manufacturer"`
		ModelName    string `xml:"modelName"`
		ModelNumber  string `xml:"modelNumber"`
		ModelDesc    string `xml:"modelDescription"`
		Presentation string `xml:"presentationURL"`
	} `xml:"device"`
}

func (dm *DiscoveryManager) scanSSDP(localIP string, results map[string]*DiscoveredDevice) {
	dm.setProgress("UPnP/SSDP 扫描中...")

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Printf("[discovery] SSDP listen failed: %v", err)
		return
	}
	defer conn.Close()

	destAddr, _ := net.ResolveUDPAddr("udp4", ssdpAddr)
	conn.WriteTo(ssdpDiscoverMsg, destAddr)

	done := time.After(3 * time.Second)
	buf := make([]byte, 2048)
	for {
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break
		}

		ssdpResp := parseSSDPResponse(buf[:n])
		if ssdpResp == nil || ssdpResp.Location == "" {
			continue
		}

		locIP := extractIPFromURL(ssdpResp.Location)
		if locIP == "" || locIP == localIP {
			continue
		}

		if isCameraSSDP(ssdpResp) {
			dm.mu.Lock()
			if _, exists := results[locIP]; !exists {
				dev := &DiscoveredDevice{
					IP:         locIP,
					Method:     "upnp",
					Confidence: 30,
					HTTPURL:    ssdpResp.Location,
				}
				server := strings.ToLower(ssdpResp.Server)
				for _, fp := range cameraFingerprints {
					if strings.Contains(server, strings.ToLower(fp.Brand)) {
						dev.Brand = fp.Brand
						dev.BrandCN = fp.BrandCN
						dev.Confidence = 40
						break
					}
				}
				results[locIP] = dev
				log.Printf("[discovery] SSDP found: %s (server=%s)", locIP, ssdpResp.Server)
			}
			dm.mu.Unlock()
		}

		select {
		case <-done:
			return
		default:
		}
	}

	// Fetch descriptions
	for ip, dev := range results {
		if dev.Method == "upnp" && dev.HTTPURL != "" {
			desc := fetchUPnPDescription(dev.HTTPURL)
			if desc != nil {
				dev.Brand = desc.Device.Manufacturer
				dev.Model = desc.Device.ModelName + " " + desc.Device.ModelNumber
				if desc.Device.FriendlyName != "" && dev.Brand == "" {
					dev.Brand = desc.Device.Manufacturer
				}
				dev.Confidence = 60
				dev.DeviceType = "camera"
				log.Printf("[discovery] UPnP desc for %s: %s %s", ip, desc.Device.Manufacturer, desc.Device.ModelName)
			}
		}
	}
}

func isCameraSSDP(resp *ssdpResponse) bool {
	st := strings.ToLower(resp.ST)
	server := strings.ToLower(resp.Server)
	cameraSTs := []string{"ipcamera", "camera", "video", "media", "onvif", "device"}
	for _, s := range cameraSTs {
		if strings.Contains(st, s) {
			return true
		}
	}
	cameraBrands := []string{"hikvision", "dahua", "tp-link", "ezviz", "uniview",
		"xiongmai", "axis", "bosch", "panasonic", "xiaomi", "huawei"}
	for _, b := range cameraBrands {
		if strings.Contains(server, b) {
			return true
		}
	}
	return false
}

func fetchUPnPDescription(url string) *upnpDeviceDesc {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var desc upnpDeviceDesc
	if err := xml.NewDecoder(resp.Body).Decode(&desc); err != nil {
		return nil
	}
	return &desc
}

// ============ ONVIF WS-Discovery Scanner ============

const onvifDiscoveryAddr = "239.255.255.250:3702"

var onvifProbeMsg = []byte(
	`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
  xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
  xmlns:wsd="http://schemas.xmlsoap.org/ws/2005/04/discovery"
  xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
  <soap:Header>
    <wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</wsa:Action>
    <wsa:MessageID>uuid:9a00e1e0-1a23-4b89-9a0a-8a9a0a9a0a0a</wsa:MessageID>
    <wsa:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</wsa:To>
  </soap:Header>
  <soap:Body>
    <wsd:Probe>
      <wsd:Types>dn:NetworkVideoTransmitter</wsd:Types>
    </wsd:Probe>
  </soap:Body>
</soap:Envelope>`)

func (dm *DiscoveryManager) scanONVIF(localIP string, results map[string]*DiscoveredDevice) {
	dm.setProgress("ONVIF 扫描中...")

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Printf("[discovery] ONVIF listen failed: %v", err)
		return
	}
	defer conn.Close()

	destAddr, _ := net.ResolveUDPAddr("udp4", onvifDiscoveryAddr)
	conn.WriteTo(onvifProbeMsg, destAddr)

	done := time.After(3 * time.Second)
	buf := make([]byte, 8192)
	for {
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break
		}

		data := buf[:n]
		if !bytes.Contains(data, []byte("ProbeMatches")) {
			continue
		}

		xaddrs := extractXMLContent(data, "XAddrs")
		for _, xaddr := range xaddrs {
			ip := extractIPFromURL(xaddr)
			if ip == "" || ip == localIP {
				continue
			}

			dm.mu.Lock()
			if existing, ok := results[ip]; ok {
				existing.Method = "onvif"
				if existing.Brand == "" {
					existing.Brand = extractONVIFBrand(data)
				}
				existing.Confidence = max(existing.Confidence, 70)
				existing.Protocol = "onvif"
				existing.HTTPURL = xaddr
				log.Printf("[discovery] ONVIF confirmed: %s (%s)", ip, existing.Brand)
			} else {
				dev := &DiscoveredDevice{
					IP:         ip,
					Method:     "onvif",
					Brand:      extractONVIFBrand(data),
					Confidence: 70,
					Protocol:   "onvif",
					HTTPURL:    xaddr,
				}
				results[ip] = dev
				log.Printf("[discovery] ONVIF found: %s", ip)
			}
			dm.mu.Unlock()
		}

		select {
		case <-done:
			return
		default:
		}
	}
}

func extractONVIFBrand(data []byte) string {
	scopes := extractXMLContent(data, "Scopes")
	for _, scope := range scopes {
		scope = strings.ToLower(scope)
		for _, fp := range cameraFingerprints {
			if strings.Contains(scope, strings.ToLower(fp.Brand)) {
				return fp.Brand
			}
		}
	}
	return ""
}

// ============ TCP Port Scanner ============

var cameraPorts = []int{554, 80, 8080, 5000, 8554, 443, 1935, 37777, 34567, 8899, 8000, 9000, 9001, 9090}

func (dm *DiscoveryManager) scanPorts(localIP string, arpTable map[string]string, results map[string]*DiscoveredDevice) {
	dm.setProgress("端口扫描中...")

	subnet, err := getLANSubnet(localIP)
	if err != nil {
		log.Printf("[discovery] cannot determine subnet: %v", err)
		return
	}

	arpIPs := make([]string, 0, len(arpTable))
	for ip := range arpTable {
		if ip != localIP {
			arpIPs = append(arpIPs, ip)
		}
	}

	portScanResults := make(map[string]map[int]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Scan ARP hosts first
	for _, ip := range arpIPs {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			openPorts := scanIPPorts(ip)
			if len(openPorts) > 0 {
				mu.Lock()
				portScanResults[ip] = openPorts
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()

	// Quick subnet scan if few ARP hosts
	if len(arpIPs) < 10 {
		dm.setProgress("全子网扫描中...")
		ipChan := make(chan string, 50)
		var scanWg sync.WaitGroup
		for i := 0; i < 10; i++ {
			scanWg.Add(1)
			go func() {
				defer scanWg.Done()
				for targetIP := range ipChan {
					mu.Lock()
					_, alreadyFound := portScanResults[targetIP]
					mu.Unlock()
					if alreadyFound {
						continue
					}
					openPorts := scanIPPorts(targetIP)
					if len(openPorts) > 0 {
						mu.Lock()
						portScanResults[targetIP] = openPorts
						mu.Unlock()
					}
				}
			}()
		}

		base := net.ParseIP(subnet).To4()
		if base != nil {
			for i := 1; i < 255; i++ {
				testIP := net.IP(make([]byte, 4))
				copy(testIP, base)
				testIP[3] = byte(i)
				ipStr := testIP.String()
				if ipStr == localIP {
					continue
				}
				select {
				case ipChan <- ipStr:
				default:
				}
			}
		}
		close(ipChan)
		scanWg.Wait()
	}

	// Merge results
	mu.Lock()
	for ip, ports := range portScanResults {
		dm.mu.Lock()
		dev, exists := results[ip]
		dm.mu.Unlock()

		hasRTSP := false
		hasHTTP := false
		hasONVIF := false
		for p := range ports {
			switch p {
			case 554, 8554:
				hasRTSP = true
			case 80, 8080, 443:
				hasHTTP = true
			case 5000:
				hasONVIF = true
			}
		}

		if !exists {
			dev = &DiscoveredDevice{
				IP:         ip,
				Ports:      sortPorts(ports),
				Method:     "portscan",
				Confidence: 10,
			}
			if mac, ok := arpTable[ip]; ok {
				dev.MAC = mac
				brand, brandCN, devType := lookupMACVendor(mac)
				dev.MACVendor = brand
				if brandCN != "" {
					dev.BrandCN = brandCN
					dev.Brand = brand
					dev.Confidence = 30
				}
				// Set device type from MAC if not multi-category brand
				if devType != "" && devType != "multi" {
					dev.DeviceType = devType
				} else if devType == "multi" {
					dev.DeviceType = "unknown" // will be refined by port/HTTP scan
				}
			}
			if hasONVIF {
				dev.Protocol = "onvif"
				dev.Confidence = max(dev.Confidence, 40)
			} else if hasRTSP {
				dev.Protocol = "rtsp"
				dev.Confidence = max(dev.Confidence, 25)
			} else if hasHTTP {
				dev.Protocol = "http"
			}

			dm.mu.Lock()
			results[ip] = dev
			dm.mu.Unlock()
		} else {
			dev.Ports = sortPorts(ports)
			if hasONVIF {
				dev.Protocol = "onvif"
				dev.Confidence = max(dev.Confidence, 50)
			} else if hasRTSP && dev.Confidence < 30 {
				dev.Protocol = "rtsp"
				dev.Confidence = max(dev.Confidence, 30)
			}
		}
	}
	mu.Unlock()
}

func scanIPPorts(ip string) map[int]bool {
	ports := make(map[int]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, port := range cameraPorts {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			addr := fmt.Sprintf("%s:%d", ip, p)
			conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
			if err != nil {
				return
			}
			conn.Close()
			mu.Lock()
			ports[p] = true
			mu.Unlock()
		}(port)
	}
	wg.Wait()
	return ports
}

// ============ HTTP Fingerprinting ============

func (dm *DiscoveryManager) fingerprintHTTP(results map[string]*DiscoveredDevice) {
	dm.setProgress("HTTP 指纹识别中...")

	var wg sync.WaitGroup
	for ip, dev := range results {
		httpPort := findHTTPPort(dev.Ports)
		if httpPort == 0 {
			continue
		}
		wg.Add(1)
		go func(ip string, dev *DiscoveredDevice, port int) {
			defer wg.Done()
			fingerprintDeviceHTTP(ip, dev, port)
		}(ip, dev, httpPort)
	}
	wg.Wait()
}

func fingerprintDeviceHTTP(ip string, dev *DiscoveredDevice, port int) {
	url := fmt.Sprintf("http://%s:%d/", ip, port)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body := make([]byte, 8192)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])
	bodyLower := strings.ToLower(bodyStr)

	serverHeader := resp.Header.Get("Server")
	serverLower := strings.ToLower(serverHeader)

	for _, fp := range cameraFingerprints {
		if fp.Brand == "Generic" {
			continue
		}
		matched := false

		if strings.Contains(serverLower, strings.ToLower(fp.Brand)) {
			matched = true
		}

		if fp.HTTPTitle != "" {
			titleMatch := regexp.MustCompile(`<title>(.*?)</title>`)
			matches := titleMatch.FindStringSubmatch(bodyStr)
			if len(matches) > 1 && strings.Contains(strings.ToLower(matches[1]), strings.ToLower(fp.HTTPTitle)) {
				matched = true
			}
		}

		if fp.HTTPCheck != "" && strings.Contains(bodyLower, strings.ToLower(fp.HTTPCheck)) {
			matched = true
		}

		if matched {
			if dev.Confidence < 60 {
				dev.Brand = fp.Brand
				dev.BrandCN = fp.BrandCN
				dev.Confidence = 60
				dev.DeviceType = "camera"
				log.Printf("[discovery] HTTP fingerprint %s → %s", ip, fp.Brand)
			}
			return
		}
	}

	cameraPatterns := []string{
		"/doc/page/login.asp", "/login.asp", "/index.asp",
		"onvif", "cgi-bin", "snapshot", "Streaming",
		"webClient.html", "web/client.html", "live.html",
		"activeX", "netviewer",
	}
	for _, pattern := range cameraPatterns {
		if strings.Contains(bodyLower, pattern) {
			if dev.Confidence < 35 {
				dev.Brand = "Unknown Camera"
				dev.BrandCN = "未知摄像头"
				dev.Confidence = 35
				dev.DeviceType = "camera"
				log.Printf("[discovery] Likely camera: %s (pattern: %s)", ip, pattern)
			}
			break
		}
	}
}

// ============ RTSP Path Guessing ============

func (dm *DiscoveryManager) guessRTSPPaths(results map[string]*DiscoveredDevice) {
	dm.setProgress("RTSP 路径探测中...")

	var wg sync.WaitGroup
	for ip, dev := range results {
		if dev.Confidence < 10 {
			continue
		}
		rtspPort := findRTSPPort(dev.Ports)
		if rtspPort == 0 {
			continue
		}
		wg.Add(1)
		go func(ip string, dev *DiscoveredDevice, port int) {
			defer wg.Done()
			probeRTSPPaths(ip, dev, port)
		}(ip, dev, rtspPort)
	}
	wg.Wait()
}

func probeRTSPPaths(ip string, dev *DiscoveredDevice, port int) {
	var fp *CameraFingerprint
	for i, f := range cameraFingerprints {
		if strings.EqualFold(f.Brand, dev.Brand) || (dev.Brand == "" && f.Brand == "Generic") {
			fp = &cameraFingerprints[i]
			break
		}
	}
	if fp == nil {
		fp = &cameraFingerprints[len(cameraFingerprints)-1]
	}

	for _, path := range fp.RTSPPaths {
		url := fmt.Sprintf("rtsp://%s:%d%s", ip, port, path)
		if verifyRTSP(url) {
			dev.RTSPURLs = append(dev.RTSPURLs, url)
			if dev.Confidence < 80 {
				dev.Confidence = 80
			}
			if dev.Protocol != "onvif" {
				dev.Protocol = "rtsp"
			}
			log.Printf("[discovery] RTSP verified: %s → %s", url, dev.Brand)
			return
		}
	}
}

func verifyRTSP(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-rtsp_transport", "tcp",
		"-i", url,
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "v:0",
		"-timeout", "3000000",
	)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return bytes.Contains(output, []byte("\"codec_name\""))
}

// ============ Device Type Inference ============

// inferDeviceTypeFromPorts determines device type based on open ports
func inferDeviceTypeFromPorts(ports []int) string {
	has := func(p int) bool {
		for _, v := range ports {
			if v == p {
				return true
			}
		}
		return false
	}

	// Camera: RTSP + HTTP is strongest signal
	if has(554) && (has(80) || has(443) || has(8080)) {
		return "camera"
	}
	if has(554) {
		return "camera"
	}
	// Phone IP Webcam (non-standard ports): 8554+RSTP + 8080 HTTP
	if has(8554) && (has(8080) || has(80)) {
		return "camera"
	}
	if has(8554) {
		return "camera"
	}
	// ONVIF
	if has(5000) {
		return "camera"
	}
	// RTMP server possible
	if has(1935) && (has(80) || has(8080)) {
		return "camera"
	}
	// Router: HTTP admin + no RTSP
	if has(80) || has(443) {
		return "router"
	}
	// Printer: usually port 80 + 9100 (JetDirect)
	if has(80) && has(9100) {
		return "printer"
	}
	if has(9100) {
		return "printer"
	}
	// NAS: SMB (445) + HTTP (80/5000/5001)
	if has(445) && (has(80) || has(443) || has(5000) || has(5001)) {
		return "nas"
	}
	if has(5000) || has(5001) {
		return "nas"
	}
	// Display (TV): HDMI-CEC usually via 8899
	if has(8899) {
		return "display"
	}
	// AC: often port 80 with specific HTTP patterns (detected later)
	// USB camera
	return "unknown"
}

func (dm *DiscoveryManager) inferDeviceTypes(results map[string]*DiscoveredDevice) {
	dm.setProgress("设备类型推断中...")
	for _, dev := range results {
		// If already determined by MAC and not multi, keep it
		if dev.DeviceType != "" && dev.DeviceType != "unknown" {
			continue
		}

		// Try port-based inference
		if len(dev.Ports) > 0 {
			portType := inferDeviceTypeFromPorts(dev.Ports)
			if portType != "unknown" {
				dev.DeviceType = portType
				continue
			}
		}

		// For multi-category brands, use port fingerprint
		if dev.Brand != "" && multiCategoryBrands[dev.Brand] {
			if len(dev.Ports) > 0 {
				pt := inferDeviceTypeFromPorts(dev.Ports)
				dev.DeviceType = pt
				// Override Xiaomi routers: most Xiaomi boxes with 80+554 are cameras
				if dev.Brand == "Xiaomi" && dev.DeviceType == "router" && hasPort(dev.Ports, 554) {
					dev.DeviceType = "camera"
				}
				continue
			}
		}

		// If UPnP discovered with a URL, it's likely a camera or router
		if dev.Method == "upnp" {
			if dev.Confidence >= 30 {
				dev.DeviceType = "camera"
			} else {
				dev.DeviceType = "router"
			}
		}

		if dev.DeviceType == "" {
			dev.DeviceType = "unknown"
		}
	}
}

func hasPort(ports []int, p int) bool {
	for _, v := range ports {
		if v == p {
			return true
		}
	}
	return false
}

func (dm *DiscoveryManager) scanUSB(results map[string]*DiscoveredDevice) {
	dm.setProgress("USB 摄像头检测中...")
	for i := 0; i < 10; i++ {
		devPath := fmt.Sprintf("/dev/video%d", i)
		if _, err := os.Stat(devPath); err == nil {
			if isVideoCaptureDevice(devPath) {
				dev := &DiscoveredDevice{
					IP:         devPath,
					HWAddr:     devPath,
					Method:     "usb",
					Confidence: 90,
					Protocol:   "usb",
					DeviceType: "camera",
					Brand:      "USB Camera",
					BrandCN:    "USB摄像头",
				}
				key := "usb:" + devPath
				if _, exists := results[key]; !exists {
					results[key] = dev
					log.Printf("[discovery] USB camera: %s", devPath)
				}
			}
		}
	}
}

func isVideoCaptureDevice(devPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "v4l2-ctl",
		"--device", devPath, "--all")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	outStr := strings.ToLower(string(out))
	return strings.Contains(outStr, "video capture") || strings.Contains(outStr, "yuyv") || strings.Contains(outStr, "mjpeg")
}

// ============ Main Scan Orchestrator ============

func (dm *DiscoveryManager) StartScan(localIP string) error {
	dm.mu.Lock()
	if dm.scanning {
		dm.mu.Unlock()
		return fmt.Errorf("scan already in progress")
	}
	dm.scanning = true
	dm.progress = "初始化..."
	dm.results = make(map[string]*DiscoveredDevice)
	dm.stopCh = make(chan struct{})
	dm.mu.Unlock()

	go func() {
		results := make(map[string]*DiscoveredDevice)

		// Phase 1: ARP table
		arpTable := getARPTable()
		log.Printf("[discovery] ARP table has %d entries", len(arpTable))

		// Phase 2: SSDP/UPnP
		dm.scanSSDP(localIP, results)

		// Phase 3: ONVIF
		dm.scanONVIF(localIP, results)

		// Phase 4: TCP port scan
		dm.scanPorts(localIP, arpTable, results)

		// Phase 5: HTTP fingerprinting
		dm.fingerprintHTTP(results)

		// Phase 6: RTSP path guessing
		dm.guessRTSPPaths(results)

		// Phase 7: Device type inference (from MAC, ports, HTTP fingerprint)
		dm.inferDeviceTypes(results)

		// Phase 8: USB cameras
		dm.scanUSB(results)

		dm.mu.Lock()
		dm.results = results
		dm.scanning = false
		dm.progress = fmt.Sprintf("扫描完成，发现 %d 个设备", len(results))
		dm.mu.Unlock()

		log.Printf("[discovery] Scan complete. Found %d devices", len(results))
	}()

	return nil
}

func (dm *DiscoveryManager) StopScan() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if dm.scanning && dm.stopCh != nil {
		close(dm.stopCh)
		dm.scanning = false
		dm.progress = "已停止"
	}
}

func (dm *DiscoveryManager) GetProgress() DiscoveryProgress {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return DiscoveryProgress{
		Scanning: dm.scanning,
		Progress: dm.progress,
		Results:  dm.results,
	}
}

func (dm *DiscoveryManager) setProgress(p string) {
	dm.mu.Lock()
	dm.progress = p
	dm.mu.Unlock()
}

// ============ Helpers ============

func extractIPFromURL(url string) string {
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "rtsp://")
	host := strings.Split(url, "/")[0]
	host = strings.Split(host, ":")[0]
	return host
}

func getLANSubnet(localIP string) (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
				continue
			}
			if ipnet.IP.String() == localIP {
				mask := ipnet.Mask
				base := ipnet.IP.Mask(mask)
				return base.String(), nil
			}
		}
	}
	return "", fmt.Errorf("could not find subnet for %s", localIP)
}

func extractXMLContent(data []byte, tag string) []string {
	var results []string
	openTag := []byte("<" + tag + ">")
	closeTag := []byte("</" + tag + ">")
	for {
		start := bytes.Index(data, openTag)
		if start == -1 {
			break
		}
		start += len(openTag)
		end := bytes.Index(data[start:], closeTag)
		if end == -1 {
			break
		}
		results = append(results, string(data[start:start+end]))
		data = data[start+end+len(closeTag):]
	}
	return results
}

func findHTTPPort(ports []int) int {
	for _, p := range ports {
		if p == 80 || p == 8080 || p == 443 {
			return p
		}
	}
	if len(ports) > 0 {
		return ports[0]
	}
	return 0
}

func findRTSPPort(ports []int) int {
	for _, p := range ports {
		if p == 554 {
			return p
		}
	}
	for _, p := range ports {
		if p == 8554 {
			return p
		}
	}
	if len(ports) > 0 {
		return ports[0]
	}
	return 0
}

func sortPorts(m map[int]bool) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
