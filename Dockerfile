FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_REVISION=unknown
WORKDIR /src
COPY go.mod *.go ./
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-X main.buildRevision=$BUILD_REVISION -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o /streamforge .

FROM docker.io/library/debian:trixie-slim
ARG PUID=1000
ARG PGID=1000
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid "$PGID" streamforge \
    && useradd --uid "$PUID" --gid "$PGID" --no-create-home --shell /usr/sbin/nologin streamforge
COPY --from=build /streamforge /usr/local/bin/streamforge
WORKDIR /working
USER streamforge
ENTRYPOINT ["/usr/local/bin/streamforge"]
