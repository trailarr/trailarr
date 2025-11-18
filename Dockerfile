# Build stage

# Build React frontend
FROM node:20 AS react-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web .
RUN npm run build

# Build Go backend
FROM golang:1.25.1 AS go-builder
WORKDIR /app
COPY . .
COPY --from=react-builder /app/web/dist /app/web/dist
RUN make build

# Final image
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates python3 python3-pip wget xz-utils \
    && rm -rf /var/lib/apt/lists/* \
    && wget -O - https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-lgpl.tar.xz \
     | tar -xJ -C /app/bin --strip-components=1 --wildcards '*/ffmpeg' '*/ffprobe' \
    && pip3 install --no-cache-dir yt-dlp curl_cffi

WORKDIR /app
COPY --from=go-builder /app/bin/trailarr /app/bin/trailarr

EXPOSE 8080
CMD ["/app/bin/trailarr"]
