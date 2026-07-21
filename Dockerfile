# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26 AS build
WORKDIR /src

# Dependencies first (better layer caching).
COPY go.mod go.sum ./
RUN go mod download

# Generated code (templ/sqlc) is committed, so the image build needs no codegen tools.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/familyplanner ./cmd/server

# ---- runtime stage ----
# debian-slim (not distroless) so the video widget can shell out to yt-dlp +
# ffmpeg to download YouTube videos for ad-free local playback.
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg python3 curl \
    && curl -fsSL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp \
    && chmod a+rx /usr/local/bin/yt-dlp \
    && apt-get purge -y curl && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/familyplanner /familyplanner
EXPOSE 8080
ENV FP_ADDR=:8080 \
    FP_DATA_DIR=/data \
    FP_TIMEZONE=Europe/Brussels \
    FP_LOCALE=nl
VOLUME ["/data"]
ENTRYPOINT ["/familyplanner"]
