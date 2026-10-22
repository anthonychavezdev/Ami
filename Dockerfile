# syntax=docker/dockerfile:1
# One image for the browser app and the API: ami-server serves the web build
# from /web and the API from the same origin, so nothing is baked into the
# page. Built and run by compose.yaml; docs/docker.md has the commands.
#
#   docker build -t ami-server .                      the runtime image (what compose builds)
#   docker build --target artifacts -o dist .         just dist/ami-server and dist/web, no Go or Node needed
#   docker build --platform linux/arm64 ...           either, for another CPU; the build stages run natively

# The browser build. A production page talks to the origin it was loaded
# from (mobile/src/lib/config.ts), so it needs no server address here. The
# export is the same for every CPU, so this stage runs on the build machine.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /app
ENV CI=1 EXPO_NO_TELEMETRY=1
COPY mobile/package.json mobile/package-lock.json ./
RUN npm ci --ignore-scripts
COPY mobile/ ./
RUN npx expo export --platform web --output-dir /web

# The server: a static binary, like `build server`, cross-compiled for the
# target CPU from the build machine.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /ami-server .

# Just the artifacts: `docker build --target artifacts -o dist .` leaves
# dist/ami-server and dist/web on disk, for a server that is not run in Docker.
FROM scratch AS artifacts
COPY --from=server /ami-server /ami-server
COPY --from=web /web /web

# What runs: the binary, the web build, its own user, the media volume, with
# Alpine's packages upgraded so a floating tag never keeps a fixed CVE.
# Certificates are for the push notifications it sends to Expo.
FROM alpine:3.21 AS runtime
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates tzdata && adduser -D -H -u 1000 ami
COPY --from=server /ami-server /usr/local/bin/ami-server
COPY --from=web /web /web
RUN mkdir -p /data/media && chown ami /data/media
USER ami
VOLUME /data/media
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=40s CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["ami-server"]
CMD ["-addr", ":8080", "-media-dir", "/data/media", "-web-dir", "/web"]
