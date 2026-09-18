// xm_sdk_bridge.c — standalone C bridge, no SDK header dependency
#include <stdlib.h>
#include <string.h>
#include <stdint.h>

// Manual declarations of SDK functions (no header needed)
extern long long H264_DVR_Login(char* ip, unsigned short port, char* user, char* pass,
    void* devInfo, int* error, int socketType);
extern int H264_DVR_Logout(long long loginID);
extern long long H264_DVR_Init(void* cb, unsigned long user);
extern void H264_DVR_SetConnectTime(long waitTime, long tryTimes);
extern long H264_DVR_GetDevConfig(long long loginID, unsigned long cmd, int channel,
    char* outBuf, unsigned long outSize, unsigned long* retSize, int timeout);
extern long H264_DVR_SetDevConfig(long long loginID, unsigned long cmd, int channel,
    char* inBuf, unsigned long inSize, int timeout);

long long xm_init() {
    return H264_DVR_Init(0, 0);
}

void xm_set_connect_time(long waitMs, long tries) {
    H264_DVR_SetConnectTime(waitMs, tries);
}

	long long xm_login(const char* ip, int port, const char* user, const char* pass, int* outChannels, int* outError) {
    char ipBuf[64], userBuf[64], passBuf[64];
    strncpy(ipBuf, ip, 63); ipBuf[63] = 0;
    strncpy(userBuf, user, 63); userBuf[63] = 0;
    strncpy(passBuf, pass, 63); passBuf[63] = 0;
    
    unsigned char devInfo[512];
    int nError = 0;
    long long loginID = H264_DVR_Login(ipBuf, (unsigned short)port, userBuf, passBuf, devInfo, &nError, 0);
    if (outError) *outError = nError;
    if (loginID > 0 && outChannels) {
        // byChanNum is at offset 56 in H264_DVR_DEVICEINFO (typically)
        *outChannels = devInfo[56];
    }
    return loginID;
}

int xm_logout(long long loginID) {
    return H264_DVR_Logout(loginID) ? 0 : -1;
}

int xm_get_color(long long loginID, int channel, int* brightness, int* contrast, int* saturation, int* sharpness) {
    unsigned char buf[4096];
    unsigned long retSize = 0;
    int ret = H264_DVR_GetDevConfig(loginID, 557, channel, (char*)buf, sizeof(buf), &retSize, 2000);
    if (ret != 1 || retSize < 36) return -1;
    int* params = (int*)(buf + 8);
    *brightness = params[0];
    *contrast = params[1];
    *saturation = params[2];
    if (sharpness) *sharpness = params[6];
    return 0;
}

int xm_set_color(long long loginID, int channel, int brightness, int contrast, int saturation, int sharpness) {
    unsigned char buf[36] = {0};
    int* params = (int*)(buf + 8);
    params[0] = brightness;
    params[1] = contrast;
    params[2] = saturation;
    params[6] = sharpness;
    int ret = H264_DVR_SetDevConfig(loginID, 557, channel, (char*)buf, sizeof(buf), 2000);
    return (ret == 1) ? 0 : -1;
}
