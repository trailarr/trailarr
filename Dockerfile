# Build stage

# Build React frontend
FROM node:24 AS react-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web .
RUN npm run build

# Build Go backend
FROM golang:1.25.1 AS go-builder
WORKDIR /app
## Copy module files first so Docker can cache module download step
## Copy module files and sources. Note: combining these weakens layer cache
## for module downloads but keeps the Dockerfile shorter and simpler.
COPY go.mod go.sum cmd internal web/embed.go ./

# Copy built frontend assets from the react builder stage
COPY --from=react-builder /app/web/dist /app/web/dist

RUN make build

# Final image
FROM ubuntu:22.04
## unzip is required by the Deno installer; xz-utils is required to unpack
## yt-dlp/ffmpeg release assets at runtime (see internal/system_update.go).
RUN apt-get update && apt-get install -y ca-certificates wget unzip xz-utils \
    && rm -rf /var/lib/apt/lists/*

## yt-dlp requires an external JavaScript runtime for full YouTube extraction
## (nsig/signature solving). Deno is the only runtime yt-dlp enables by default,
## so putting it on PATH is enough for yt-dlp to detect it without extra flags.
## The official installer shells out to curl, so curl is installed only for this
## step and purged afterwards (the Go binary serves HTTP at runtime).
## DENO_VERSION is pinned for reproducible builds and DENO_INSTALL places the
## binary in /usr/local/bin.
ARG DENO_VERSION=v2.9.7
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl \
    && curl -fsSL https://deno.land/install.sh | CI=1 DENO_INSTALL=/usr/local sh -s "${DENO_VERSION}" \
    && deno --version \
    && apt-get purge -y --auto-remove curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=go-builder /app/bin/trailarr /app/bin/trailarr
## Create a non-root user and set permissions for the final image to avoid
## running the service as root. This improves safety when the container runs.
RUN groupadd -r trailarr || true && \
    useradd -r -g trailarr -d /nonexistent -s /usr/sbin/nologin trailarr || true && \
    mkdir -p /app && chown -R trailarr:trailarr /app && chmod 0755 /app/bin/trailarr
USER trailarr

EXPOSE 8080
CMD ["/app/bin/trailarr"]
