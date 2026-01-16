FROM golang:1.23-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /irr-monitor ./cmd/irr-monitor

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

RUN adduser -D -g '' appuser

RUN mkdir -p /data && chown appuser:appuser /data

WORKDIR /app

COPY --from=builder /irr-monitor .

USER appuser
VOLUME ["/data"]

ENV STATE_FILE=/data/state.json
ENV POLL_INTERVAL=60

CMD ["./irr-monitor"]
