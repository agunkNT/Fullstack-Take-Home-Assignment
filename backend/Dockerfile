# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy all source files
COPY . .

# Ensure dependencies and checksums are completely resolved
RUN go mod tidy

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server main.go

# Production stage
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]
