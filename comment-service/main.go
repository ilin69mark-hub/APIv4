package main

import (
	"context"
	"database/sql"
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
	_ "github.com/mattn/go-sqlite3"
	"github.com/google/uuid"
)

// Comment структура комментария
type Comment struct {
	ID        int       `json:"id"`
	NewsID    int       `json:"news_id"`
	ParentID  *int      `json:"parent_id,omitempty"`
	Text      string    `json:"text"`
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
	DBPath string
}

func main() {
	config := &Config{
		Port: getEnv("COMMENT_SERVICE_PORT", "8081"),
		DBPath: getEnv("DB_PATH", "./comments.db"),
	}

	// Инициализация базы данных
	db, err := initDB(config.DBPath)
	if err != nil {
		log.Fatalf("Ошибка инициализации базы данных: %v", err)
	}
	defer db.Close()

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
	r.Post("/comments", CreateCommentHandler(db))
	r.Get("/comments", GetCommentsHandler(db))
	r.Delete("/comments/{id}", DeleteCommentHandler(db))

	server := &http.Server{
		Addr:    ":" + config.Port,
		Handler: r,
	}

	// Запуск сервера в горутине
	go func() {
		log.Printf("Comment Service запущен на порту %s", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	// Ожидание сигнала остановки
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Завершение работы Comment Service...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Ошибка завершения работы сервера: %v", err)
	}
	log.Println("Comment Service успешно остановлен")
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

// initDB инициализация базы данных
func initDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// Создание таблицы комментариев
	query := `
	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		news_id INTEGER NOT NULL,
		parent_id INTEGER,
		text TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (parent_id) REFERENCES comments (id)
	);
	CREATE INDEX IF NOT EXISTS idx_news_id ON comments (news_id);
	CREATE INDEX IF NOT EXISTS idx_parent_id ON comments (parent_id);
	`
	
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// CreateCommentHandler обработчик создания комментария
func CreateCommentHandler(db *sql.DB) http.HandlerFunc {
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

		// Валидация входных данных
		if req.NewsID <= 0 {
			http.Error(w, "NewsID должен быть положительным числом", http.StatusBadRequest)
			return
		}

		if len(req.Text) == 0 {
			http.Error(w, "Текст комментария не может быть пустым", http.StatusBadRequest)
			return
		}

		if len(req.Text) > 1000 { // Максимальная длина текста
			http.Error(w, "Текст комментария слишком длинный", http.StatusBadRequest)
			return
		}

		// Проверка существования parent_id
		if req.ParentID != nil {
			var exists bool
			err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM comments WHERE id = ?)", *req.ParentID).Scan(&exists)
			if err != nil || !exists {
				http.Error(w, "Указанный parent_id не существует", http.StatusBadRequest)
				return
			}
		}

		// Вставка комментария в базу данных
		query := "INSERT INTO comments (news_id, parent_id, text) VALUES (?, ?, ?)"
		result, err := db.Exec(query, req.NewsID, req.ParentID, req.Text)
		if err != nil {
			http.Error(w, "Ошибка сохранения комментария в базу данных", http.StatusInternalServerError)
			return
		}

		id, err := result.LastInsertId()
		if err != nil {
			http.Error(w, "Ошибка получения ID нового комментария", http.StatusInternalServerError)
			return
		}

		// Получение созданного комментария
		var comment Comment
		err = db.QueryRow("SELECT id, news_id, parent_id, text, created_at FROM comments WHERE id = ?", id).
			Scan(&comment.ID, &comment.NewsID, &comment.ParentID, &comment.Text, &comment.CreatedAt)
		if err != nil {
			http.Error(w, "Ошибка получения созданного комментария", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{
			Status: "success",
			Data:   comment,
		})
	}
}

// GetCommentsHandler обработчик получения комментариев
func GetCommentsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		newsIDStr := r.URL.Query().Get("news_id")
		if newsIDStr == "" {
			http.Error(w, "Параметр news_id обязателен", http.StatusBadRequest)
			return
		}

		newsID, err := strconv.Atoi(newsIDStr)
		if err != nil {
			http.Error(w, "Неверный формат параметра news_id", http.StatusBadRequest)
			return
		}

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page <= 0 {
			page = 1
		}

		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if pageSize <= 0 {
			pageSize = 10
		}

		// Подсчет общего количества комментариев для пагинации
		var total int
		err = db.QueryRow("SELECT COUNT(*) FROM comments WHERE news_id = ?", newsID).Scan(&total)
		if err != nil {
			http.Error(w, "Ошибка подсчета комментариев", http.StatusInternalServerError)
			return
		}

		// Получение комментариев с пагинацией
		query := "SELECT id, news_id, parent_id, text, created_at FROM comments WHERE news_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?"
		rows, err := db.Query(query, newsID, pageSize, (page-1)*pageSize)
		if err != nil {
			http.Error(w, "Ошибка получения комментариев", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var comments []Comment
		for rows.Next() {
			var comment Comment
			err := rows.Scan(&comment.ID, &comment.NewsID, &comment.ParentID, &comment.Text, &comment.CreatedAt)
			if err != nil {
				http.Error(w, "Ошибка сканирования комментария", http.StatusInternalServerError)
				return
			}
			comments = append(comments, comment)
		}

		pageCount := (total + pageSize - 1) / pageSize

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{
			Status: "success",
			Data:   comments,
			Pagination: &Pagination{
				Page:      page,
				PageSize:  pageSize,
				Total:     total,
				PageCount: pageCount,
			},
		})
	}
}

// DeleteCommentHandler обработчик удаления комментария
func DeleteCommentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Неверный ID комментария", http.StatusBadRequest)
			return
		}

		// Проверка существования комментария
		var exists bool
		err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM comments WHERE id = ?)", id).Scan(&exists)
		if err != nil || !exists {
			http.Error(w, "Комментарий не найден", http.StatusNotFound)
			return
		}

		// Удаление комментария
		_, err = db.Exec("DELETE FROM comments WHERE id = ?", id)
		if err != nil {
			http.Error(w, "Ошибка удаления комментария", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Response{
			Status: "success",
			Data:   fmt.Sprintf("Комментарий с ID %d удален", id),
		})
	}
}

// getEnv вспомогательная функция для получения переменных окружения
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}