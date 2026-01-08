package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// Config конфигурация сервиса
type Config struct {
	Port string
}

// CensorService структура сервиса цензуры
type CensorService struct {
	bannedWords map[string]bool
	regex       *regexp.Regexp
}

// NewCensorService создает новый экземпляр сервиса цензуры
func NewCensorService() *CensorService {
	// Инициализация списка запрещенных слов (можно расширить через конфиг)
	bannedWords := map[string]bool{
		"qwerty": true,
		"йцукен": true,
		"zxvbnm": true,
		// Добавьте сюда больше запрещенных слов при необходимости
	}

	// Создание регулярного выражения для поиска запрещенных слов
	// Объединяем все слова в один паттерн
	var patterns []string
	for word := range bannedWords {
		patterns = append(patterns, regexp.QuoteMeta(word))
	}
	
	regex, err := regexp.Compile(strings.Join(patterns, "|"))
	if err != nil {
		log.Fatalf("Ошибка компиляции регулярного выражения: %v", err)
	}

	return &CensorService{
		bannedWords: bannedWords,
		regex:       regex,
	}
}

// CheckRequest структура запроса для проверки текста
type CheckRequest struct {
	Text string `json:"text"`
}

// Response структура для унифицированного ответа
type Response struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func main() {
	config := &Config{
		Port: getEnv("CENSOR_SERVICE_PORT", "8082"),
	}

	censorService := NewCensorService()

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(LoggerMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(TimeoutMiddleware(30 * time.Second))

	// Health check endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{Status: "ok"})
	})

	// Маршрут API
	r.Post("/check", censorService.CheckHandler)

	server := &http.Server{
		Addr:    ":" + config.Port,
		Handler: r,
	}

	// Запуск сервера в горутине
	go func() {
		log.Printf("Censor Service запущен на порту %s", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	// Ожидание сигнала остановки
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Завершение работы Censor Service...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Ошибка завершения работы сервера: %v", err)
	}
	log.Println("Censor Service успешно остановлен")
}

// LoggerMiddleware логирование запросов
func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		
		requestID := r.Context().Value(middleware.RequestIDKey)
		if requestID == nil {
			requestID = uuid.New().String()
		}

		log.Printf("[%s] %s %s %s", requestID, r.Method, r.URL.Path, r.RemoteAddr)

		next.ServeHTTP(ww, r)

		log.Printf("[%s] %s %s %d %v", requestID, r.Method, r.URL.Path, ww.Status(), time.Since(start))
	})
}

// TimeoutMiddleware middleware для таймаута запросов
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// CheckHandler обработчик проверки текста на запрещенные слова
func (cs *CensorService) CheckHandler(w http.ResponseWriter, r *http.Request) {
	var req CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Неверный формат тела запроса", http.StatusBadRequest)
		return
	}

	// Проверка текста на наличие запрещенных слов (регистронезависимо)
	if cs.isBanned(req.Text) {
		http.Error(w, "Текст содержит запрещенные слова", http.StatusBadRequest)
		return
	}

	// Если текст прошел проверку
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Status: "success",
		Data:   "Текст прошел проверку",
	})
}

// isBanned проверяет, содержит ли текст запрещенные слова
func (cs *CensorService) isBanned(text string) bool {
	// Приведение текста к нижнему регистру для регистронезависимой проверки
	lowerText := strings.ToLower(text)
	
	// Проверка с помощью регулярного выражения
	return cs.regex.MatchString(lowerText)
}

// getEnv вспомогательная функция для получения переменных окружения
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}