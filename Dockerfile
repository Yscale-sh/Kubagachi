# Multi-stage, multi-arch build for kubagachi (kubekritters).
# The browser UI is embedded into the Go binary via go:embed (web/dist), so the
# web assets are built first, then baked into the binary.
#
# Buildx cross-builds: the web + Go stages run on the NATIVE builder platform
# ($BUILDPLATFORM) and cross-compile / fetch for the requested $TARGETARCH, so a
# single `docker buildx build --platform linux/amd64,linux/arm64` produces both
# without QEMU-emulating the toolchains. The runtime stage is pulled per target.

# 1. Build the web UI (Vite) → web/dist. Static output, arch-independent, so run
#    it once on the native builder regardless of the target platform.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Build the Go binary with the freshly built web/dist embedded. Cross-compile
#    for $TARGETOS/$TARGETARCH (CGO off → pure static cross-build).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
# kubectl powers the in-pod exec shell (internal/app/web.go → `kubectl exec -it`).
# Pinned to match the cluster's Kubernetes minor; override with --build-arg.
ARG KUBECTL_VERSION=v1.31.5
WORKDIR /src
# Fetch kubectl for the TARGET arch (buildx sets TARGETARCH per requested
# platform) so the arm64 image ships an arm64 kubectl, not an amd64 one.
RUN apk add --no-cache curl \
 && mkdir -p /out \
 && curl -fsSLo /out/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/${TARGETOS}/${TARGETARCH}/kubectl" \
 && chmod +x /out/kubectl
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags='-s -w' -o /out/kubagachi ./cmd/kubagachi

# 3. Minimal runtime: binary + critter sprites + entrypoint. No --platform here,
#    so buildx pulls alpine for each target arch automatically.
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
