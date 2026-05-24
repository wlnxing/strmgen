FROM --platform=$BUILDPLATFORM docker.io/library/node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.22-alpine AS go-build
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/strm ./cmd/strm

FROM docker.io/library/caddy:2-alpine
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=go-build /out/strm /app/strm
COPY --from=web-build /src/web/dist /srv/strm-web
COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY deploy/entrypoint.sh /entrypoint.sh
ENV STRM_LISTEN_ADDR=127.0.0.1:18080 \
    STRM_DB_PATH=/data/strm.db \
    STRM_CRON_TZ=Asia/Shanghai
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
