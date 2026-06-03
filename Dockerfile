ARG GO_VERSION=1.26.3
FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -v -buildvcs=false -o /run-app ./cmd/intervals-mcp


FROM debian:bookworm

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /run-app /usr/local/bin/

RUN useradd --system --uid 10001 --home-dir /nonexistent --shell /usr/sbin/nologin appuser
USER appuser

CMD ["run-app"]
