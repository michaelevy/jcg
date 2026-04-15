# Stage 1: Build
FROM golang:1.25-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=1 required for mattn/go-sqlite3.
# Dynamic linking against glibc — runtime image must also be glibc-based (debian).
RUN CGO_ENABLED=1 GOOS=linux go build -o server ./cmd/server

# Stage 2: Run
FROM debian:bookworm-slim

WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/server .

VOLUME ["/data"]
EXPOSE 8080
CMD ["./server", \
     "-db=file:/data/jcg.db?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", \
     "-addr=:8080"]
