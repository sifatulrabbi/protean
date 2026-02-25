# --- Build stage ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

ENV GOENV=production

# Cache deps before copying source
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/vfs-server ./cmd/vfs-server

# --- Runtime stage ---
FROM alpine:3.23.3

WORKDIR /app

ENV GOENV=production

COPY --from=builder /app/bin/vfs-server .

EXPOSE 8090

ENTRYPOINT ["/app/vfs-server"]
