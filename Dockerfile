FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod *.go ./
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /streamforge .

FROM alpine:3.23
ARG PUID=1000
ARG PGID=1000
RUN addgroup -g "$PGID" streamforge && adduser -D -H -u "$PUID" -G streamforge streamforge
COPY --from=build /streamforge /usr/local/bin/streamforge
WORKDIR /working
USER streamforge
ENTRYPOINT ["/usr/local/bin/streamforge"]
