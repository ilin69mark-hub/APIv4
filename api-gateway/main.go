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
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

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

// Comment структура комментария
type Comment struct {
	ID        int       `json:"id"`
	NewsID    int       `json:"news_id"`
	ParentID  *int      `json:"parent_id,omitempty"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// News структура новости
type News struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	Comments    []Comment `json:"comments,omitempty"`
}

// Config конфигурация сервиса
type Config struct {
	Port             string
	CommentServiceURL string
	CensorServiceURL  string
	NewsServiceURL    string
}

func main() {
	config := &Config{
		Port:             getEnv("API_GATEWAY_PORT", "8080"),
		CommentServiceURL: getEnv("COMMENT_SERVICE_URL", "http://comment-service:8081"),
		CensorServiceURL:  getEnv("CENSOR_SERVICE_URL", "http://censor-service:8082"),
		NewsServiceURL:    getEnv("NEWS_SERVICE_URL", "http://news-aggregator:8083"),
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
	r.Get("/news", GetNewsHandler(config))
	r.Get("/news/{id}", GetNewsWithCommentsHandler(config))
	r.Post("/comment", CreateCommentHandler(config))

	server := &http.Server{
		Addr:    ":" + config.Port,
		Handler: r,
	}

	// Запуск сервера в горутине
	go func() {
		log.Printf("API Gateway запущен на порту %s", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	// Ожидание сигнала остановки
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Завершение работы API Gateway...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Ошибка завершения работы сервера: %v", err)
	}
	log.Println("API Gateway успешно остановлен")
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
func GetNewsHandler(config *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page <= 0 {
			page = 1
		}
		
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if pageSize <= 0 {
			pageSize = 10
		}
		
		search := r.URL.Query().Get("search")

		// Запрос к сервису новостей
		client := &http.Client{Timeout: 10 * time.Second}
		url := fmt.Sprintf("%s/news?page=%d&page_size=%d", config.NewsServiceURL, page, pageSize)
		if search != "" {
			url += "&search=" + search
		}
		
		resp, err := client.Get(url)
		if err != nil {
			http.Error(w, fmt.Sprintf("Ошибка получения новостей: %v", err), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		var newsResponse Response
		if err := json.NewDecoder(resp.Body).Decode(&newsResponse); err != nil {
			http.Error(w, "Ошибка декодирования ответа от сервиса новостей", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(newsResponse)
	}
}

// GetNewsWithCommentsHandler обработчик получения новости с комментариями
func GetNewsWithCommentsHandler(config *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		newsIDStr := chi.URLParam(r, "id")
		newsID, err := strconv.Atoi(newsIDStr)
		if err != nil {
			http.Error(w, "Неверный ID новости", http.StatusBadRequest)
			return
		}

		// Параллельные запросы к сервису новостей и комментариев
		type result struct {
			news     *News
			comments []Comment
			err      error
		}
		
		ch := make(chan result, 2)
		
		// Запрос новости
		go func() {
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(fmt.Sprintf("%s/news/%d", config.NewsServiceURL, newsID))
			if err != nil {
				ch <- result{err: err}
				return
			}
			defer resp.Body.Close()
			
			var newsResponse Response
			if err := json.NewDecoder(resp.Body).Decode(&newsResponse); err != nil {
				ch <- result{err: err}
				return
			}
			
			if news, ok := newsResponse.Data.(map[string]interface{}); ok {
				newsObj := &News{
					ID:        int(news["id"].(float64)),
					Title:     news["title"].(string),
					Content:   news["content"].(string),
					CreatedAt: time.Now(), // в реальности дата придет из сервиса новостей
				}
				
				// Преобразуем дату создания если она есть
				if createdAt, ok := news["created_at"].(string); ok {
					if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
						newsObj.CreatedAt = t
					}
				}
				
				ch <- result{news: newsObj}
			} else {
				ch <- result{err: fmt.Errorf("неверный формат данных новости")}
			}
		}()
		
		// Запрос комментариев
		go func() {
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(fmt.Sprintf("%s/comments?news_id=%d", config.CommentServiceURL, newsID))
			if err != nil {
				ch <- result{err: err}
				return
			}
			defer resp.Body.Close()
			
			var commentsResponse Response
			if err := json.NewDecoder(resp.Body).Decode(&commentsResponse); err != nil {
				ch <- result{err: err}
				return
			}
			
			var comments []Comment
			if data, ok := commentsResponse.Data.([]interface{}); ok {
				for _, c := range data {
					if commentMap, ok := c.(map[string]interface{}); ok {
						newComment := Comment{
							ID:      int(commentMap["id"].(float64)),
							NewsID:  int(commentMap["news_id"].(float64)),
							Text:    commentMap["text"].(string),
							CreatedAt: time.Now(), // в реальности дата придет из сервиса комментариев
						}
						
						// Преобразуем дату создания если она есть
						if createdAt, ok := commentMap["created_at"].(string); ok {
							if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
								newComment.CreatedAt = t
							}
						}
						
						// Обработка parent_id если он есть
						if parentID, exists := commentMap["parent_id"]; exists && parentID != nil {
							pid := int(parentID.(float64))
							newComment.ParentID = &pid
						}
						
						comments = append(comments, newComment)
					}
				}
			}
			
			ch <- result{comments: comments}
		}()
		
		// Сбор результатов
		var news *News
		var comments []Comment
		
		for i := 0; i < 2; i++ {
			res := <-ch
			if res.err != nil {
				http.Error(w, fmt.Sprintf("Ошибка получения данных: %v", res.err), http.StatusInternalServerError)
				return
			}
			
			if res.news != nil {
				news = res.news
			} else if res.comments != nil {
				comments = res.comments
			}
		}
		
		// Привязка комментариев к новости
		if news != nil {
			news.Comments = comments
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{
			Status: "success",
			Data:   news,
		})
	}
}

// CreateCommentHandler обработчик создания комментария
func CreateCommentHandler(config *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			NewsID   int    `json:"news_id"`
			ParentID *int   `json:"parent_id,omitempty"`
			Text     string `json:"text"`
		}
		
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Неверный формат тела запроса", http.StatusBadRequest)
			return
		}
		
		// Проверка текста на запрещенные слова через CensorService
		client := &http.Client{Timeout: 10 * time.Second}
		censorReq := map[string]string{"text": req.Text}
		censorReqBytes, _ := json.Marshal(censorReq)
		
		resp, err := client.Post(
			fmt.Sprintf("%s/check", config.CensorServiceURL),
			"application/json",
			strings.NewReader(string(censorReqBytes)),
		)
		if err != nil {
			http.Error(w, "Ошибка проверки текста", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			http.Error(w, "Текст содержит запрещенные слова", http.StatusBadRequest)
			return
		}
		
		// Отправка комментария в CommentService
		commentReq := map[string]interface{}{
			"news_id":   req.NewsID,
			"text":      req.Text,
			"parent_id": req.ParentID,
		}
		commentReqBytes, _ := json.Marshal(commentReq)
		
		resp, err = client.Post(
			fmt.Sprintf("%s/comments", config.CommentServiceURL),
			"application/json",
			strings.NewReader(string(commentReqBytes)),
		)
		if err != nil {
			http.Error(w, "Ошибка сохранения комментария", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			http.Error(w, "Ошибка сохранения комментария", resp.StatusCode)
			return
		}
		
		var commentResp Response
		if err := json.NewDecoder(resp.Body).Decode(&commentResp); err != nil {
			http.Error(w, "Ошибка декодирования ответа от сервиса комментариев", http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(commentResp)
	}
}

// getEnv вспомогательная функция для получения переменных окружения
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}