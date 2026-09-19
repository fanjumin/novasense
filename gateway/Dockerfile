# =============================================
# Dockerfile — NovaSense 网关 v0.8.4
# =============================================
# Builds: Go backend (with XM SDK cgo) + ffmpeg + wapa-pull
# MediaMTX runs as a separate container (see docker-compose.yml)
# glibc-based stages: libxmnetsdk.so requires glibc (not musl/alpine)
# =============================================

# Stage 1: Build Go binary (glibc + cgo)
FROM golang:1.26-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev && rm -rf /var/lib/apt/lists/*

ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o novasense-gateway . \
 && CGO_ENABLED=0 go build -ldflags="-s -w" -o wapa-pull ./cmd/wapa-pull

# Stage 2: Runtime (glibc, ships XM SDK)
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates tzdata libstdc++6 && rm -rf /var/lib/apt/lists/*

# Backend binary
COPY --from=builder /build/novasense-gateway /usr/local/bin/

# XM (雄迈 XiongMai) SDK runtime
COPY --from=builder /build/sdk/lib/libxmnetsdk.so /usr/local/lib/
ENV LD_LIBRARY_PATH=/usr/local/lib

# WAPA protocol helper (optional, for 波粒 cameras) — built from ./cmd/wapa-pull
COPY --from=builder /build/wapa-pull /usr/local/bin/wapa-pull

# Frontend files
COPY frontend/ /app/frontend/

# MediaMTX config (shared with mediamtx container)
COPY mediamtx/mediamtx.yml /app/mediamtx/mediamtx.yml

RUN mkdir -p /app/data

# GAP-A: pigo 人脸级联必须进运行镜像 — 放 /app/backend/ 因 /app/data 会被卷挂载遮蔽
# (findCascadeFile 候选1 "backend/facefinder" 相对 WORKDIR=/app 命中此路径)
RUN mkdir -p /app/backend
COPY --from=builder /build/facefinder /app/backend/facefinder

ENV DATA_DIR=/app/data

EXPOSE 8899

WORKDIR /app
CMD ["novasense-gateway"]
