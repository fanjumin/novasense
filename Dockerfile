# =============================================
# Dockerfile — video-stream-manager v0.8.3
# =============================================
# Builds: Go backend + ffmpeg + wapa-pull
# MediaMTX runs as a separate container (see docker-compose.yml)
# =============================================

# Stage 1: Build Go binary
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o video-stream-manager .

# Stage 2: Runtime
FROM alpine:3.20

RUN apk add --no-cache ffmpeg=7.1-r0 ca-certificates tzdata

# Backend binary
COPY --from=builder /build/video-stream-manager /usr/local/bin/

# WAPA protocol helper (optional, for 波力 cameras)
COPY bin/wapa-pull /usr/local/bin/wapa-pull

# Frontend files
COPY frontend/ /app/frontend/

# MediaMTX config (shared with mediamtx container)
COPY mediamtx/mediamtx.yml /app/mediamtx/mediamtx.yml

RUN mkdir -p /app/data

ENV DATA_DIR=/app/data

EXPOSE 8899

WORKDIR /app
CMD ["video-stream-manager"]
