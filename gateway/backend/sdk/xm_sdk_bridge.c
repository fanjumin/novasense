// xm_sdk_bridge.c — C bridge between Go and XiongMai NetSDK
#include <stdlib.h>
#include <string.h>

// The SDK header needs these for C compilation
#define H264_DVR_API
#define CALL_METHOD
typedef int BOOL;
typedef unsigned int DWORD;
typedef int LONG;
typedef unsigned short WORD;
typedef long long LONG64;
typedef unsigned long ULONG;

#include "netsdk.h"

// Login to device, returns loginID (>0 = success)
long long xm_login(const char* ip, int port, const char* user, const char* pass, int* outChannels) {
    H264_DVR_DEVICEINFO devInfo;
    int nError = 0;
    char ipBuf[64], userBuf[64], passBuf[64];
    strncpy(ipBuf, ip, 63);
    strncpy(userBuf, user, 63);
    strncpy(passBuf, pass, 63);
    
    long long loginID = H264_DVR_Login(ipBuf, (unsigned short)port, userBuf, passBuf, &devInfo, &nError, 0);
    if (loginID > 0 && outChannels) {
        *outChannels = devInfo.byChanNum;
    }
    return loginID;
}

// Logout from device
int xm_logout(long long loginID) {
    return H264_DVR_Logout(loginID) ? 0 : -1;
}

// Get video color config, returns 0 on success
int xm_get_color(long long loginID, int channel, int* brightness, int* contrast, int* saturation, int* sharpness) {
    unsigned char buf[4096];
    unsigned long retSize = 0;
    
    int ret = H264_DVR_GetDevConfig(loginID, 557, channel, (char*)buf, sizeof(buf), &retSize, 2000);
    if (ret != 1 || retSize < 36) return -1;
    
    // Skip first 8 bytes (SDK_TIMESECTION), read 7 int32s
    int* params = (int*)(buf + 8);
    *brightness = params[0];
    *contrast = params[1];
    *saturation = params[2];
    if (sharpness) *sharpness = params[6]; // nAcutance
    return 0;
}

// Set video color config, returns 0 on success
int xm_set_color(long long loginID, int channel, int brightness, int contrast, int saturation, int sharpness) {
    unsigned char buf[36] = {0}; // all zeros = all-time, default values
    
    int* params = (int*)(buf + 8);
    params[0] = brightness;
    params[1] = contrast;
    params[2] = saturation;
    params[6] = sharpness;
    
    int ret = H264_DVR_SetDevConfig(loginID, 557, channel, (char*)buf, sizeof(buf), 2000);
    return (ret == 1) ? 0 : -1;
}
