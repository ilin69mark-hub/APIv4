package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

// NewsAggregatorURL — URL внешнего сервиса новостей
var NewsAggregatorURL = getEnv("NEWS_AGGREGATOR_URL", "http://news-aggregator:8083")

// CommentServiceURL — URL сервиса комментариев
var CommentServiceURL = getEnv("COMMENT_SERVICE_URL", "http://comment-service:8081")

// CensorServiceURL — URL сервиса цензуры
var CensorServiceURL = getEnv("CENSOR_SERVICE_URL", "http://censor-service:8082")

// Config — конфигурация приложения
type Config struct {
	Port string
}

// App — структура приложения
type App struct {
	config Config
	logger zerolog.Logger
	router chi.Router
}

// Response — универсальная структура ответа
type Response struct {
	Status     string      `json:"status"`
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination — структура пагинации
type Pagination struct {
	Page      int `json:"page"`
	PageSize  int `json:"page_size"`
	Total     int `json:"total"`
	PageCount int `json:"page_count"`
}

// News — структура новости
type News struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Date    string `json:"date"`
}

// Comment — структура комментария
type Comment struct {
	ID       int    `json:"id"`
	NewsID   int    `json:"news_id"`
	ParentID *int   `json:"parent_id,omitempty"`
	Text     string `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// RequestIDMiddleware — мидлвар для генерации/пропуска request_id
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateRequestID — генерирует уникальный request_id
func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// LoggerMiddleware — мидлвар для логирования запросов
func LoggerMiddleware(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return hlog.NewHandler(*logger)
}

// TimeoutMiddleware — мидлвар для установки таймаута
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// getEnv — получает значение переменной окружения или возвращает значение по умолчанию
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// NewApp — создает новое приложение
func NewApp(config Config) *App {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Recoverer)
	r.Use(RequestIDMiddleware)
	r.Use(LoggerMiddleware(&logger))
	r.Use(TimeoutMiddleware(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Request-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	app := &App{
		config: config,
		logger: logger,
		router: r,
	}

	// Routes
	r.Get("/", app.Home)
	r.Get("/health", app.HealthCheck)
	r.Get("/news", app.GetNews)
	r.Get("/news/{id}", app.GetNewsByID)
	r.Post("/comment", app.CreateComment)

	return app
}

// Home — главная страница
func (a *App) Home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "API Gateway OK")
}

// HealthCheck — проверка состояния сервиса
func (a *App) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Status: "ok"})
}

// GetNews — получение списка новостей с пагинацией и поиском
func (a *App) GetNews(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	search := r.URL.Query().Get("search")

	// Валидация параметров
	if len(search) > 100 {
		a.sendError(w, http.StatusBadRequest, "Search query too long")
		return
	}

	// Формирование URL для запроса к News Aggregator
	u, err := url.Parse(NewsAggregatorURL + "/news")
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to parse news aggregator URL")
		return
	}
	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	q.Set("page_size", strconv.Itoa(pageSize))
	if search != "" {
		q.Set("search", search)
	}
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to fetch news")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to read news response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		a.sendError(w, resp.StatusCode, string(body))
		return
	}

	var newsResponse Response
	if err := json.Unmarshal(body, &newsResponse); err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to parse news response")
		return
	}

	a.sendResponse(w, http.StatusOK, newsResponse.Data, &Pagination{
		Page:     page,
		PageSize: pageSize,
		Total:    100, // В реальном приложении это должно приходить из News Aggregator
		PageCount: 10, // В реальном приложении это должно приходить из News Aggregator
	})
}

// GetNewsByID — получение новости по ID с комментариями
func (a *App) GetNewsByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	newsID, err := strconv.Atoi(id)
	if err != nil || newsID < 1 {
		a.sendError(w, http.StatusBadRequest, "Invalid news ID")
		return
	}

	// Запрос деталей новости
	newsURL := fmt.Sprintf("%s/news/%d", NewsAggregatorURL, newsID)
	newsResp, err := http.Get(newsURL)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to fetch news details")
		return
	}
	defer newsResp.Body.Close()

	newsBody, err := io.ReadAll(newsResp.Body)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to read news details")
		return
	}

	if newsResp.StatusCode != http.StatusOK {
		a.sendError(w, newsResp.StatusCode, string(newsBody))
		return
	}

	var newsResponse Response
	if err := json.Unmarshal(newsBody, &newsResponse); err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to parse news details")
		return
	}

	// Запрос комментариев
	commentsURL := fmt.Sprintf("%s/comments?news_id=%d", CommentServiceURL, newsID)
	commentsResp, err := http.Get(commentsURL)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to fetch comments")
		return
	}
	defer commentsResp.Body.Close()

	commentsBody, err := io.ReadAll(commentsResp.Body)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to read comments")
		return
	}

	if commentsResp.StatusCode != http.StatusOK {
		a.sendError(w, commentsResp.StatusCode, string(commentsBody))
		return
	}

	var commentsResponse Response
	if err := json.Unmarshal(commentsBody, &commentsResponse); err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to parse comments")
		return
	}

	// Агрегация результатов
	result := map[string]interface{}{
		"news":      newsResponse.Data,
		"comments":  commentsResponse.Data,
	}

	a.sendResponse(w, http.StatusOK, result, nil)
}

// CreateComment — создание комментария
func (a *App) CreateComment(w http.ResponseWriter, r *http.Request) {
	var comment Comment
	if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
		a.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Проверка текста на наличие запрещённых слов
	censorURL := CensorServiceURL + "/check"
	censorPayload := map[string]string{"text": comment.Text}
	censorBody, err := json.Marshal(censorPayload)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to marshal censor request")
		return
	}

	resp, err := http.Post(censorURL, "application/json", strings.NewReader(string(censorBody)))
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to check comment for censorship")
		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		a.sendError(w, http.StatusBadRequest, "Comment contains forbidden words")
		return
	}

	// Отправка комментария в Comment Service
	commentsURL := CommentServiceURL + "/comments"
	commentsBody, err := json.Marshal(comment)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to marshal comment")
		return
	}

	resp, err = http.Post(commentsURL, "application/json", strings.NewReader(string(commentsBody)))
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to create comment")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to read comment response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		a.sendError(w, resp.StatusCode, string(body))
		return
	}

	var commentResponse Response
	if err := json.Unmarshal(body, &commentResponse); err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to parse comment response")
		return
	}

	a.sendResponse(w, http.StatusOK, commentResponse.Data, nil)
}

// sendResponse — отправляет успешный JSON-ответ
func (a *App) sendResponse(w http.ResponseWriter, statusCode int, data interface{}, pagination *Pagination) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(Response{
		Status:     "success",
		Data:       data,
		Pagination: pagination,
	})
}

// sendError — отправляет JSON-ответ с ошибкой
func (a *App) sendError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(Response{
		Status: "error",
		Error:  message,
	})
}

// Run — запускает HTTP-сервер
func (a *App) Run() error {
	return http.ListenAndServe(":"+a.config.Port, a.router)
}

func main() {
	config := Config{
		Port: getEnv("PORT", "8080"),
	}

	app := NewApp(config)

	log.Printf("API Gateway запущен на порту %s", config.Port)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}