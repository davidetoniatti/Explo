FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

# Set the working directory
WORKDIR /app

# Copy go.mod/go.sum first and download modules so this layer is cached
# and skipped on source-only changes.
COPY go.mod go.sum ./
RUN go mod download

# Copy the Go source code into the container
COPY ./ .

# Build the Go binary based on the target architecture
ARG TARGETARCH
ARG VERSION=dev
RUN GOOS=linux GOARCH=$TARGETARCH go build -ldflags "-X explo/src/config.Version=${VERSION}" -o explo ./src/main/

FROM python:3.12-alpine

# Install runtime deps: libc compat, ffmpeg, yt-dlp, tzdata, shadow for user management, su-exec for user switching
RUN apk add --no-cache \
    libc6-compat \
    ffmpeg \
    yt-dlp \
    tzdata \
    shadow \
    su-exec 

# Install ytmusicapi in the container, pinned to a known-good version
RUN pip install --no-cache-dir ytmusicapi==1.10.0

# Set working directory
WORKDIR /opt/explo/

# Copy entrypoint, binary, python helper
COPY ./docker/start.sh /start.sh
COPY --from=builder /app/explo .
COPY src/downloader/youtube_music/search_ytmusic.py .

RUN chmod +x /start.sh ./explo

CMD ["/start.sh"]