# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate swagger docs if swag is available
RUN go install github.com/swaggo/swag/cmd/swag@latest && \
    swag init -g cmd/api/main.go -o docs || true

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/api ./cmd/api

# Run stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/bin/api /app/api
COPY --from=builder /app/docs /app/docs

EXPOSE 8080

CMD ["/app/api"]
