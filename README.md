# Микросервисная архитектура на Go

Этот проект представляет собой микросервисную архитектуру, состоящую из трех сервисов:
1. API Gateway - единая точка входа для клиентов
2. Comment Service - управление комментариями
3. Censor Service - проверка комментариев на запрещенные слова

## Архитектурная схема

```
Клиент → APIGateway → [CensorService] → CommentService
                    ↘ [NewsAggregator*] ↗
```

* NewsAggregator предполагается внешним сервисом (реализован как заглушка)

## Технологии

- Go (Golang)
- Chi Router
- PostgreSQL/SQLite
- Docker
- Docker Compose

## Структура проекта

```
/workspace/
├── api-gateway/
├── comment-service/
├── censor-service/
├── news-aggregator/
├── docker-compose.yml
└── Makefile
```

## Запуск проекта

### Локальный запуск

```bash
make build
make run
```

### Docker Compose

```bash
docker-compose up --build
```

## API endpoints

### API Gateway (порт 8080)

- `GET /news` - получение списка новостей
- `GET /news/{id}` - получение новости с комментариями
- `POST /comment` - создание комментария

### Comment Service (порт 8081)

- `POST /comments` - создание комментария
- `GET /comments?news_id=X` - получение комментариев по новости
- `DELETE /comments/{id}` - удаление комментария

### Censor Service (порт 8082)

- `POST /check` - проверка текста на запрещенные слова

## Конфигурация

Конфигурация сервисов осуществляется через переменные окружения.

Быстрый старт 
make build — собрать бинарники Go.
make docker-build — создать Docker-образы.
make docker-run — запустить всё в контейнерах.

Исправлены существенные недостатки. Версия 4

