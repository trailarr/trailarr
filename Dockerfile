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
RUN apt-get update && apt-get install -y ca-certificates wget xz-utils \
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
