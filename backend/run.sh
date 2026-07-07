#!/bin/bash
export DATA_DIR=/home/deployuser/projects/video-stream-manager/data
cd /home/deployuser/projects/video-stream-manager/backend
exec ./video-stream-manager >> /tmp/vsm-stdout.log 2>> /tmp/vsm-stderr.log
