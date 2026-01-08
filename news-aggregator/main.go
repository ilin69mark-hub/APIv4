package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// News структура новости
type News struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Response структура для унифицированного ответа
type Response struct {
	Status     string      `json:"status"`
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination структура для пагинации
type Pagination struct {
	Page      int `json:"page"`
	PageSize  int `json:"page_size"`
	Total     int `json:"total"`
	PageCount int `json:"page_count"`
}

// Config конфигурация сервиса
type Config struct {
	Port string
}

func main() {
	config := &Config{
		Port: getEnv("NEWS_SERVICE_PORT", "8083"),
	}

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

	// Маршруты API
	r.Get("/news", GetNewsHandler)
	r.Get("/news/{id}", GetNewsByIDHandler)

	server := &http.Server{
		Addr:    ":" + config.Port,
		Handler: r,
	}

	// Запуск сервера в горутине
	go func() {
		log.Printf("News Aggregator запущен на порту %s", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	// Ожидание сигнала остановки
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Завершение работы News Aggregator...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Ошибка завершения работы сервера: %v", err)
	}
	log.Println("News Aggregator успешно остановлен")
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

// GetNewsHandler обработчик получения списка новостей
func GetNewsHandler(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize <= 0 {
		pageSize = 10
	}
	
	search := r.URL.Query().Get("search")

	// Фиктивные данные для новостей
	allNews := []News{
		{
			ID:        1,
			Title:     "Новость 1",
			Content:   "Это содержание первой новости",
			CreatedAt: time.Date(2023, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			ID:        2,
			Title:     "Новость 2",
			Content:   "Это содержание второй новости",
			CreatedAt: time.Date(2023, 1, 16, 14, 45, 0, 0, time.UTC),
		},
		{
			ID:        3,
			Title:     "Новость 3",
			Content:   "Это содержание третьей новости",
			CreatedAt: time.Date(2023, 1, 17, 9, 20, 0, 0, time.UTC),
		},
		{
			ID:        4,
			Title:     "Новость 4",
			Content:   "Это содержание четвертой новости",
			CreatedAt: time.Date(2023, 1, 18, 16, 10, 0, 0, time.UTC),
		},
		{
			ID:        5,
			Title:     "Новость 5",
			Content:   "Это содержание пятой новости",
			CreatedAt: time.Date(2023, 1, 19, 11, 5, 0, 0, time.UTC),
		},
	}

	// Фильтрация по поисковому запросу
	var filteredNews []News
	if search != "" {
		for _, news := range allNews {
			if containsIgnoreCase(news.Title, search) || containsIgnoreCase(news.Content, search) {
				filteredNews = append(filteredNews, news)
			}
		}
	} else {
		filteredNews = allNews
	}

	// Пагинация
	start := (page - 1) * pageSize
	if start >= len(filteredNews) {
		start = len(filteredNews)
	}
	
	end := start + pageSize
	if end > len(filteredNews) {
		end = len(filteredNews)
	}

	paginatedNews := filteredNews[start:end]
	total := len(filteredNews)
	pageCount := (total + pageSize - 1) / pageSize

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Status: "success",
		Data:   paginatedNews,
		Pagination: &Pagination{
			Page:      page,
			PageSize:  pageSize,
			Total:     total,
			PageCount: pageCount,
		},
	})
}

// GetNewsByIDHandler обработчик получения новости по ID
func GetNewsByIDHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Неверный ID новости", http.StatusBadRequest)
		return
	}

	// Фиктивные данные для новостей
	news := []News{
		{
			ID:        1,
			Title:     "Новость 1",
			Content:   "Это содержание первой новости",
			CreatedAt: time.Date(2023, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			ID:        2,
			Title:     "Новость 2",
			Content:   "Это содержание второй новости",
			CreatedAt: time.Date(2023, 1, 16, 14, 45, 0, 0, time.UTC),
		},
		{
			ID:        3,
			Title:     "Новость 3",
			Content:   "Это содержание третьей новости",
			CreatedAt: time.Date(2023, 1, 17, 9, 20, 0, 0, time.UTC),
		},
		{
			ID:        4,
			Title:     "Новость 4",
			Content:   "Это содержание четвертой новости",
			CreatedAt: time.Date(2023, 1, 18, 16, 10, 0, 0, time.UTC),
		},
		{
			ID:        5,
			Title:     "Новость 5",
			Content:   "Это содержание пятой новости",
			CreatedAt: time.Date(2023, 1, 19, 11, 5, 0, 0, time.UTC),
		},
	}

	for _, n := range news {
		if n.ID == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Response{
				Status: "success",
				Data:   n,
			})
			return
		}
	}

	http.Error(w, "Новость не найдена", http.StatusNotFound)
}

// containsIgnoreCase проверяет, содержит ли строка подстроку без учета регистра
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// getEnv вспомогательная функция для получения переменных окружения
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}