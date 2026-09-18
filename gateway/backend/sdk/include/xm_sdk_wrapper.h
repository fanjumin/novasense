// xm_sdk_wrapper.h — C-compatible wrapper for XiongMai NetSDK
#ifndef XM_SDK_WRAPPER_H
#define XM_SDK_WRAPPER_H

#include <stdbool.h>
#include <stdint.h>

// Override SDK macros for C compilation
#define H264_DVR_API
#define CALL_METHOD

// Define types used by SDK
typedef int32_t BOOL;
typedef uint32_t DWORD;
typedef int32_t LONG;
typedef uint16_t WORD;
typedef uint64_t LONG64;
typedef uint32_t ULONG;

// Include the SDK header with our overrides
#include "netsdk.h"

#endif
