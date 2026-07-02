# ── Build stage ────────────────────────────────
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate swagger docs
RUN go install github.com/swaggo/swag/cmd/swag@latest && \
    swag init -g cmd/server/main.go -o docs

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /chunked-upload ./cmd/server

# ── Runtime stage ───────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /chunked-upload /chunked-upload
COPY --from=builder /app/docs /docs

EXPOSE 8080
ENTRYPOINT ["/chunked-upload"]
