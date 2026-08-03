FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git=2.49.1-r0

WORKDIR /build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /entrypoint ./cmd/entrypoint

FROM cm2network/steamcmd:root

LABEL maintainer="faudil"
LABEL org.opencontainers.image.description="Project Zomboid Dedicated Server Docker"
LABEL org.opencontainers.image.source="https://github.com/faudil/project-zomboid-server-docker"

ARG DEPOT_DOWNLOADER_VERSION=3.4.0
ARG DEPOT_DOWNLOADER_SHA256=a999dec66b4850fc961bd50366696d23c2d0fad7b18790e6a5647b2f19097a53

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        lib32gcc-s1=14.2.0-19 \
        lib32stdc++6=14.2.0-19 \
        ca-certificates=20250419 \
        tzdata=2025b-4+deb13u1 \
        unzip=6.0-29 && \
    rm -rf /var/lib/apt/lists/*

# DepotDownloader replaces SteamCMD's app_update, which Steam's backend
# intermittently rejects for anonymous sessions. Its anonymous download path
# works reliably. The self-contained build needs no .NET runtime and runs
# with DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1 (set by the entrypoint).
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN curl -sL "https://github.com/SteamRE/DepotDownloader/releases/download/DepotDownloader_${DEPOT_DOWNLOADER_VERSION}/DepotDownloader-linux-x64.zip" -o /tmp/dd.zip && \
    echo "${DEPOT_DOWNLOADER_SHA256}  /tmp/dd.zip" | sha256sum -c - && \
    unzip -q /tmp/dd.zip -d /tmp/dd && \
    install -m 0755 /tmp/dd/DepotDownloader /usr/local/bin/depotdownloader && \
    rm -rf /tmp/dd /tmp/dd.zip
SHELL ["/bin/sh", "-c"]

RUN mkdir -p /home/steam/pzserver /home/steam/Zomboid/Server /home/steam/Zomboid/Saves/Multiplayer /home/steam/Zomboid/backups && \
    chown -R steam:steam /home/steam

COPY --from=builder /entrypoint /home/steam/entrypoint
RUN chmod +x /home/steam/entrypoint && chown steam:steam /home/steam/entrypoint

RUN printf '#!/bin/bash\n/home/steam/entrypoint healthcheck\n' > /healthcheck.sh && \
    chmod +x /healthcheck.sh

ENV HOME=/home/steam
ENV USER=steam

USER 1000

EXPOSE 16261/udp 16262/udp 27015/tcp 8080/tcp

VOLUME ["/home/steam/Zomboid"]
VOLUME ["/home/steam/pzserver"]

# The start period must cover the first-run server download (5-10 minutes),
# during which RCON is not yet available and the healthcheck legitimately fails.
HEALTHCHECK --interval=60s --timeout=10s --retries=3 --start-period=600s \
    CMD ["/healthcheck.sh"]

STOPSIGNAL SIGTERM

ENTRYPOINT ["/home/steam/entrypoint"]
