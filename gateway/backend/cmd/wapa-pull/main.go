// wapa-pull — pull MPEG-4 stream from WAPA (波粒) "纯数字" camera.
//
// These cameras (e.g. BL-720Q-L) wrap raw MPEG-4 video in a
// proprietary protocol on port 9001.  The raw stream lacks VOL headers
// (width/height metadata), so this program prepends a proper MPEG-4
// video object layer header for 1280x720 before outputting the clean
// elementary stream to stdout.
//
// Usage:
//
//	wapa-pull  192.0.2.10  [port=9001]
//	wapa-pull  192.0.2.10  9001 | ffmpeg -f m4v -i pipe:0 ...
package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// mpeg4StartCode is the MPEG-4 VOP (Video Object Plane) start code.
var mpeg4StartCode = []byte{0x00, 0x00, 0x01, 0xb6}

// mpeg4Header is the MPEG-4 VOL/VOS/GOV header for 1280x720 Simple Profile.
// These cameras output raw VOP data without the required headers,
// so we prepend this header to make the stream decodable by ffmpeg.
var mpeg4Header, _ = hex.DecodeString(
	"000001b001000001b58913000001000000012000c48d8800cd28045a1443" +
	"000001b24c61766335382e3133342e313030000001b300100700",
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <ip>[:port]\n", os.Args[0])
		os.Exit(1)
	}
	host := os.Args[1]
	port := 9001
	// Support "ip:port" format in a single argument
	if parts := strings.SplitN(host, ":", 2); len(parts) == 2 {
		host = parts[0]
		if v, err := strconv.Atoi(parts[1]); err == nil {
			port = v
		}
	} else if len(os.Args) > 2 {
		if v, err := strconv.Atoi(os.Args[2]); err == nil {
			port = v
		}
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	for {
		if err := stream(addr); err != nil {
			log.Printf("stream error: %v — reconnecting in 3s", err)
		}
		time.Sleep(3 * time.Second)
	}
}

func stream(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()
	log.Printf("connected to %s", addr)

	buf := make([]byte, 65536)
	synced := false
	window := make([]byte, 0, 8)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("connection closed")
			}
			return fmt.Errorf("read: %w", err)
		}
		data := buf[:n]

		if !synced {
			window = append(window, data...)
			if idx := indexOf(window, mpeg4StartCode); idx >= 0 {
				remainder := window[idx:]
				synced = true
				// Prepend MPEG-4 VOL/VOS header (raw stream lacks these)
				if _, err := os.Stdout.Write(mpeg4Header); err != nil {
					return fmt.Errorf("write header: %w", err)
				}
				if len(remainder) > 0 {
					if _, err := os.Stdout.Write(remainder); err != nil {
						return fmt.Errorf("write stdout: %w", err)
					}
				}
			} else {
				if len(window) > 64 {
					window = window[len(window)-3:]
				}
			}
			continue
		}

		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("write stdout: %w", err)
		}
	}
}

func indexOf(haystack, needle []byte) int {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
