package main

// #cgo LDFLAGS: -L${SRCDIR}/sdk/lib -lxmnetsdk -ldl
// #include <stdlib.h>
// long long xm_init();
// void xm_set_connect_time(long waitMs, long tries);
// long long xm_login(const char* ip, int port, const char* user, const char* pass, int* outChannels, int* outError);
// int xm_logout(long long loginID);
// int xm_get_color(long long loginID, int channel, int* brightness, int* contrast, int* saturation, int* sharpness);
// int xm_set_color(long long loginID, int channel, int brightness, int contrast, int saturation, int sharpness);
import "C"

import (
	"fmt"
	"log"
	"sync"
	"unsafe"
)

var (
	sdkOnce   sync.Once
	sdkInited bool
	sdkMu     sync.Mutex
)

type SDKColor struct {
	Brightness int
	Contrast   int
	Saturation int
	Sharpness  int
}

type XmDevice struct {
	LoginID  int64
	IP       string
	Channels int
}

func sdkInit() error {
	sdkMu.Lock()
	defer sdkMu.Unlock()
	if sdkInited {
		return nil
	}
	var err error
	sdkOnce.Do(func() {
		ret := int64(C.xm_init())
		if ret == 0 {
			err = fmt.Errorf("H264_DVR_Init failed")
			return
		}
		C.xm_set_connect_time(3000, 2)
		sdkInited = true
		log.Println("[sdk] initialized")
	})
	return err
}

func LoginDev(ip, user, pass string, port int) (*XmDevice, error) {
	if err := sdkInit(); err != nil {
		return nil, err
	}
	if port <= 0 {
		port = 34567
	}

	cIP := C.CString(ip)
	cUser := C.CString(user)
	cPass := C.CString(pass)
	defer C.free(unsafe.Pointer(cIP))
	defer C.free(unsafe.Pointer(cUser))
	defer C.free(unsafe.Pointer(cPass))

	var channels, nErr C.int
	loginID := int64(C.xm_login(cIP, C.int(port), cUser, cPass, &channels, &nErr))
	if loginID <= 0 {
		return nil, fmt.Errorf("login failed (err=%d)", int(nErr))
	}
	if channels == 0 {
		channels = 1
	}

	log.Printf("[sdk] logged in to %s:%d (loginID=%d, ch=%d)", ip, port, loginID, int(channels))
	return &XmDevice{LoginID: loginID, IP: ip, Channels: int(channels)}, nil
}

func LogoutDev(dev *XmDevice) {
	if dev != nil && dev.LoginID > 0 {
		C.xm_logout(C.longlong(dev.LoginID))
		log.Printf("[sdk] logged out from %s", dev.IP)
	}
}

func (dev *XmDevice) GetColor(ch int) (*SDKColor, error) {
	if ch < 0 {
		ch = 0
	}
	var b, c, s, sh C.int
	ret := int(C.xm_get_color(C.longlong(dev.LoginID), C.int(ch), &b, &c, &s, &sh))
	if ret != 0 {
		return nil, fmt.Errorf("xm_get_color failed (ret=%d)", ret)
	}
	return &SDKColor{
		Brightness: int(b), Contrast: int(c),
		Saturation: int(s), Sharpness: int(sh),
	}, nil
}

func (dev *XmDevice) SetColor(ch int, color *SDKColor) error {
	if ch < 0 {
		ch = 0
	}
	ret := int(C.xm_set_color(
		C.longlong(dev.LoginID), C.int(ch),
		C.int(color.Brightness), C.int(color.Contrast),
		C.int(color.Saturation), C.int(color.Sharpness),
	))
	if ret != 0 {
		return fmt.Errorf("xm_set_color failed (ret=%d)", ret)
	}
	log.Printf("[sdk] set color ch%d: B=%d C=%d S=%d Sh=%d",
		ch, color.Brightness, color.Contrast, color.Saturation, color.Sharpness)
	return nil
}
