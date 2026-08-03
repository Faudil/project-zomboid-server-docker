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

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        lib32gcc-s1=14.2.0-19 \
        lib32stdc++6=14.2.0-19 \
        ca-certificates=20250419 \
        tzdata=2025b-4+deb13u1 && \
    rm -rf /var/lib/apt/lists/*

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

# The start period must cover the first-run SteamCMD install (5-10 minutes),
# during which RCON is not yet available and the healthcheck legitimately fails.
HEALTHCHECK --interval=60s --timeout=10s --retries=3 --start-period=600s \
    CMD ["/healthcheck.sh"]

STOPSIGNAL SIGTERM

ENTRYPOINT ["/home/steam/entrypoint"]
