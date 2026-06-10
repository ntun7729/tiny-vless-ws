# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY main.go ./
COPY go.mod ./

# Statically compile the Go binary with optimization and stripping
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tiny-vless

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/tiny-vless .

ENV PORT=8080
ENV UUID=""
ENV PATH="/vless"

EXPOSE 8080

CMD ["./tiny-vless"]
