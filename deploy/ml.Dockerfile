# Файл: deploy/ml.Dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Собираем воркер ML
RUN go build -o /ml-worker ./cmd/ml/main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /ml-worker .
COPY .env . 

# Воркеру не нужен EXPOSE, так как он сам идет в Redis, а не ждет запросов
CMD ["./ml-worker"]