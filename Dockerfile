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
# Bundle a pinned kubectl (matches cluster Kubernetes v1.31.5). The in-pod exec
# shell (internal/app/web.go) runs `kubectl exec -it`, but the minimal runtime
# image has no package manager — so fetch the static binary here and COPY it
# into the runtime stage below.
RUN apk add --no-cache curl \
 && mkdir -p /out \
 && curl -fsSLo /out/kubectl https://dl.k8s.io/release/v1.31.5/bin/linux/amd64/kubectl \
 && chmod +x /out/kubectl
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/kubagachi ./cmd/kubagachi

# 3. Minimal runtime: binary + critter sprites + entrypoint
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /out/kubagachi /usr/local/bin/kubagachi
# kubectl powers the in-pod exec shell (web.go ExecArgs → `kubectl exec -it`).
COPY --from=build /out/kubectl /usr/local/bin/kubectl
COPY critters/ /critters/
COPY deploy/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
