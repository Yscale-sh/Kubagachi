# Multi-stage build for kubagachi (kubekritters).
# The browser UI is embedded into the Go binary via go:embed (web/dist), so the
# web assets are built first, then baked into the binary.

# 1. Build the web UI (Vite) → web/dist
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Build the Go binary with the freshly built web/dist embedded
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/kubagachi ./cmd/kubagachi

# 3. Minimal runtime: binary + critter sprites + entrypoint
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /out/kubagachi /usr/local/bin/kubagachi
COPY critters/ /critters/
COPY deploy/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
