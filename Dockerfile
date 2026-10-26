# syntax=docker/dockerfile:1
FROM node:22-alpine AS web
WORKDIR /src/web/admin
COPY web/admin/package*.json ./
RUN npm ci
COPY web/admin/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=0.1.0
ARG VCS_REF=unknown
ARG DIRTY=true
ARG CHANNEL=local
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/
RUN mkdir /out && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
    -ldflags="-X github.com/TwhomeGH/VideoServer/pkg/buildinfo.Version=$VERSION -X github.com/TwhomeGH/VideoServer/pkg/buildinfo.Commit=$VCS_REF -X github.com/TwhomeGH/VideoServer/pkg/buildinfo.Dirty=$DIRTY -X github.com/TwhomeGH/VideoServer/pkg/buildinfo.Channel=$CHANNEL -X github.com/TwhomeGH/VideoServer/pkg/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /out/ ./cmd/...

FROM alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates && addgroup -g 10001 dvn && adduser -D -u 10001 -G dvn dvn \
    && mkdir -p /data /media /app/web && chown 10001:10001 /data
WORKDIR /app
COPY --from=web /src/web/admin/dist/ /app/web/
USER 10001:10001
ENTRYPOINT ["/app/service", "-config", "/app/config.json"]

FROM runtime AS central
COPY --from=backend /out/central /app/service
COPY deployments/docker/central.json /app/config.json
EXPOSE 18620
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -q -O /dev/null http://127.0.0.1:18620/healthz || exit 1

FROM runtime AS transcode-worker
COPY --from=backend /out/transcode-worker /app/service
COPY deployments/docker/transcode-worker.json /app/config.json
EXPOSE 18630
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -q -O /dev/null http://127.0.0.1:18630/healthz || exit 1

FROM runtime AS video-node
COPY --from=backend /out/video-node /app/service
COPY deployments/docker/video-node.json /app/config.json
EXPOSE 18610
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -q -O /dev/null http://127.0.0.1:18610/healthz || exit 1
