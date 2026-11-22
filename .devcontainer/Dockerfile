FROM docker.io/golang:1.27.4-trixie

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg git \
    && rm -rf /var/lib/apt/lists/* \
    && ln -s /usr/bin/ffmpeg /usr/local/bin/ffmpeg \
    && ln -s /usr/bin/ffprobe /usr/local/bin/ffprobe \
    && mkdir -p /config /working/input /working/output

# Git settings for working on a mounted repo.
RUN git config --system core.fsmonitor false \
    && git config --system core.untrackedCache true \
    && git config --system core.preloadIndex true \
    && git config --system core.filemode false \
    && git config --system core.autocrlf input
