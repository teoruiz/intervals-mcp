# Cloudflare Containers require linux/amd64; build with
# --platform=linux/amd64 (see `make docker-build`).
ARG GO_VERSION=1.26.3
FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -buildvcs=false -trimpath -ldflags="-s -w" -o /intervals-mcp ./cmd/intervals-mcp


FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/* \
  && useradd --system --uid 10001 --home-dir /nonexistent --shell /usr/sbin/nologin appuser

COPY --from=builder /intervals-mcp /usr/local/bin/intervals-mcp

USER appuser
ENV MCP_ADDR=0.0.0.0:8080
EXPOSE 8080
CMD ["intervals-mcp"]
