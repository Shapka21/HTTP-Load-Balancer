FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o load-balancer ./cmd/HTTP-Load-Balancer/main/main.go

FROM alpine:latest

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/load-balancer /app/load-balancer
COPY config.json /app/config.json

WORKDIR /app

EXPOSE 8080

CMD ["./load-balancer", "--config", "config.json"]