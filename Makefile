# Makefile для микросервисной архитектуры

.PHONY: build test run docker-build docker-run clean

# Сборка всех сервисов
build:
	@echo "Сборка API Gateway..."
	cd api-gateway && go build -o ../bin/api-gateway main.go
	@echo "Сборка Comment Service..."
	cd comment-service && go build -o ../bin/comment-service main.go
	@echo "Сборка Censor Service..."
	cd censor-service && go build -o ../bin/censor-service main.go
	@echo "Сборка News Aggregator..."
	cd news-aggregator && go build -o ../bin/news-aggregator main.go
	@echo "Сборка завершена. Бинарные файлы находятся в папке bin/"

# Запуск тестов (заглушка - в реальном проекте нужно добавить реальные тесты)
test:
	@echo "Запуск тестов для API Gateway..."
	cd api-gateway && go test -v ./...
	@echo "Запуск тестов для Comment Service..."
	cd comment-service && go test -v ./...
	@echo "Запуск тестов для Censor Service..."
	cd censor-service && go test -v ./...
	@echo "Запуск тестов для News Aggregator..."
	cd news-aggregator && go test -v ./...

# Запуск всех сервисов (в фоне)
run: build
	@echo "Запуск всех сервисов..."
	@mkdir -p bin
	@nohup ./bin/api-gateway > api-gateway.log 2>&1 &
	@nohup ./bin/comment-service > comment-service.log 2>&1 &
	@nohup ./bin/censor-service > censor-service.log 2>&1 &
	@nohup ./bin/news-aggregator > news-aggregator.log 2>&1 &
	@echo "Сервисы запущены в фоне. Логи в *.log файлах"

# Сборка Docker образов
docker-build:
	@echo "Сборка Docker образов..."
	docker-compose build

# Запуск через Docker Compose
docker-run: docker-build
	@echo "Запуск сервисов через Docker Compose..."
	docker-compose up -d

# Остановка и удаление Docker контейнеров
docker-down:
	@echo "Остановка и удаление Docker контейнеров..."
	docker-compose down

# Очистка
clean:
	@echo "Очистка..."
	rm -rf bin/
	rm -f *.log
	rm -f */*.log
	docker-compose down -v

# Установка зависимостей для всех сервисов
deps:
	@echo "Установка зависимостей для API Gateway..."
	cd api-gateway && go mod tidy
	@echo "Установка зависимостей для Comment Service..."
	cd comment-service && go mod tidy
	@echo "Установка зависимостей для Censor Service..."
	cd censor-service && go mod tidy
	@echo "Установка зависимостей для News Aggregator..."
	cd news-aggregator && go mod tidy

# Запуск в режиме разработки с перезапуском при изменениях (требует установки air: go install github.com/cosmtrek/air@latest)
dev:
	@echo "Запуск в режиме разработки..."
	@echo "Убедитесь, что установлен air: go install github.com/cosmtrek/air@latest"
	@cd api-gateway && air &
	@cd comment-service && air &
	@cd censor-service && air &
	@cd news-aggregator && air &
	@echo "Сервисы запущены в режиме разработки"

# Проверка состояния сервисов
status:
	@echo "Проверка состояния сервисов..."
	@echo "API Gateway (порт 8080):"
	@curl -s http://localhost:8080/health || echo "API Gateway не отвечает"
	@echo "Comment Service (порт 8081):"
	@curl -s http://localhost:8081/health || echo "Comment Service не отвечает"
	@echo "Censor Service (порт 8082):"
	@curl -s http://localhost:8082/health || echo "Censor Service не отвечает"
	@echo "News Aggregator (порт 8083):"
	@curl -s http://localhost:8083/health || echo "News Aggregator не отвечает"