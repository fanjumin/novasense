# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=0 go build -o video-stream-manager -ldflags="-s -w" .

# --- runtime ---
FROM alpine:3.21

RUN apk add --no-cache ffmpeg ca-certificates tzdata

WORKDIR /app

# MediaMTX will be mounted or run separately
COPY --from=builder /build/video-stream-manager .

# Create data directory
RUN mkdir -p /app/data

EXPOSE 8899

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8899/api/health || exit 1

ENTRYPOINT ["./video-stream-manager"]
