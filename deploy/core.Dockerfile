# Файл: deploy/core.Dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app

# Копируем файлы зависимостей из корня
COPY go.mod go.sum ./
RUN go mod tidy && go mod download

# Копируем весь проект, чтобы были доступны internal и shared части
COPY . .

# Собираем именно core-сервис
RUN go build -o /core-app ./cmd/core/main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /core-app .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080
COPY migrations ./migrations
CMD ["./core-app"]