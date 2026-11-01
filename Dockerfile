# syntax=docker/dockerfile:1
FROM node:22-alpine AS web
WORKDIR /src/web/admin
COPY web/admin/package*.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
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
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY apps/ apps/
COPY internal/ internal/
COPY pkg/ pkg/
RUN mkdir /out && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
    -ldflags="-X github.com/TwhomeGH/VideoServer/pkg/buildinfo.Version=$VERSION -X github.com/TwhomeGH/VideoServer/pkg/buildinfo.Commit=$VCS_REF -X github.com/TwhomeGH/VideoServer/pkg/buildinfo.Dirty=$DIRTY -X github.com/TwhomeGH/VideoServer/pkg/buildinfo.Channel=$CHANNEL -X github.com/TwhomeGH/VideoServer/pkg/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /out/ ./apps/...

# runtime-base 不包含每次建置都會變動的 build args／labels，讓 apk 層可被重用。
FROM alpine:3.23 AS runtime-base
RUN --mount=type=cache,target=/var/cache/apk apk add --no-cache ca-certificates curl \
    && addgroup -g 10001 dvn && adduser -D -u 10001 -G dvn dvn \
    && mkdir -p /data /media /app/web && chown 10001:10001 /data
WORKDIR /app
USER 10001:10001
ENTRYPOINT ["/app/service", "-config", "/app/config.json"]

FROM runtime-base AS central
ARG VERSION=0.1.0
ARG VCS_REF=unknown
ARG DIRTY=true
ARG BUILD_TIME=unknown
ARG SOURCE_DIGEST=unknown
COPY --from=backend /out/central /app/service
COPY --from=web /src/web/admin/dist/ /app/web/
COPY deployments/docker/central.json /app/config.json
LABEL org.opencontainers.image.title="DVN" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.created=$BUILD_TIME \
      dev.dvn.dirty=$DIRTY \
      dev.dvn.source-digest=$SOURCE_DIGEST
ENV DVN_IMAGE_BUILT_AT=$BUILD_TIME \
    DVN_IMAGE_SOURCE_DIGEST=$SOURCE_DIGEST
EXPOSE 18620
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD curl -fsSk --max-time 2 -o /dev/null https://127.0.0.1:18620/healthz || curl -fsS --max-time 2 -o /dev/null http://127.0.0.1:18620/healthz || exit 1

FROM runtime-base AS transcode-worker
ARG VERSION=0.1.0
ARG VCS_REF=unknown
ARG DIRTY=true
ARG BUILD_TIME=unknown
ARG SOURCE_DIGEST=unknown
COPY --from=backend /out/transcode-worker /app/service
COPY --from=web /src/web/admin/dist/ /app/web/
COPY deployments/docker/transcode-worker.json /app/config.json
LABEL org.opencontainers.image.title="DVN" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.created=$BUILD_TIME \
      dev.dvn.dirty=$DIRTY \
      dev.dvn.source-digest=$SOURCE_DIGEST
ENV DVN_IMAGE_BUILT_AT=$BUILD_TIME \
    DVN_IMAGE_SOURCE_DIGEST=$SOURCE_DIGEST
EXPOSE 18630
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD curl -fsSk --max-time 2 -o /dev/null https://127.0.0.1:18630/healthz || curl -fsS --max-time 2 -o /dev/null http://127.0.0.1:18630/healthz || exit 1

FROM runtime-base AS video-node
USER root
RUN --mount=type=cache,target=/var/cache/apk apk add --no-cache ffmpeg
USER 10001:10001
ARG VERSION=0.1.0
ARG VCS_REF=unknown
ARG DIRTY=true
ARG BUILD_TIME=unknown
ARG SOURCE_DIGEST=unknown
COPY --from=backend /out/video-node /app/service
COPY --from=web /src/web/admin/dist/ /app/web/
COPY deployments/docker/video-node.json /app/config.json
LABEL org.opencontainers.image.title="DVN" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.created=$BUILD_TIME \
      dev.dvn.dirty=$DIRTY \
      dev.dvn.source-digest=$SOURCE_DIGEST
ENV DVN_IMAGE_BUILT_AT=$BUILD_TIME \
    DVN_IMAGE_SOURCE_DIGEST=$SOURCE_DIGEST
EXPOSE 18610
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD curl -fsSk --max-time 2 -o /dev/null https://127.0.0.1:18610/healthz || curl -fsS --max-time 2 -o /dev/null http://127.0.0.1:18610/healthz || exit 1
