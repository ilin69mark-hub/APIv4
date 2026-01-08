# Многоступенчатая сборка
FROM golang:1.24-alpine AS builder

# Установка зависимостей
RUN apk add --no-cache git

# Установка рабочей директории
WORKDIR /app

# Копирование go.mod и go.sum
COPY go.mod go.sum ./

# Загрузка зависимостей
RUN go mod download

# Копирование исходного кода
COPY main.go .

# Сборка приложения
RUN CGO_ENABLED=0 GOOS=linux go build -o api-gateway main.go

# Финальный образ
FROM alpine:latest

# Установка зависимостей для работы SQLite (если понадобится в будущем)
RUN apk --no-cache add ca-certificates

# Создание пользователя для запуска приложения
RUN adduser -D -s /bin/sh appuser

# Установка рабочей директории
WORKDIR /app

# Копирование бинарного файла из builder образа
COPY --from=builder /app/api-gateway .

# Изменение владельца файла
RUN chown appuser:appuser api-gateway

# Переключение на непривилегированного пользователя
USER appuser

# Открытие порта
EXPOSE 8080

# Запуск приложения
CMD ["./api-gateway"]