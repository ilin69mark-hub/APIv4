# Многоступенчатая сборка
FROM golang:1.21-alpine AS builder

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
RUN CGO_ENABLED=0 GOOS=linux go build -o news-aggregator main.go

# Финальный образ
FROM alpine:latest

# Установка зависимостей
RUN apk --no-cache add ca-certificates

# Создание пользователя для запуска приложения
RUN adduser -D -s /bin/sh appuser

# Установка рабочей директории
WORKDIR /app

# Копирование бинарного файла из builder образа
COPY --from=builder /app/news-aggregator .

# Изменение владельца файла
RUN chown appuser:appuser news-aggregator

# Переключение на непривилегированного пользователя
USER appuser

# Открытие порта
EXPOSE 8083

# Запуск приложения
CMD ["./news-aggregator"]