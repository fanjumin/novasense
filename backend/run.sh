#!/bin/bash
export DATA_DIR=/home/deployuser/projects/video-stream-manager/data
cd /home/deployuser/projects/video-stream-manager/backend
exec ./novasense-gateway >> /tmp/novasense-gateway-stdout.log 2>> /tmp/novasense-gateway-stderr.log
