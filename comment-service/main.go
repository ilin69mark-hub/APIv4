package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

type Config struct {
	Port string
	DBPath string
}

type App struct {
	config Config
	logger zerolog.Logger
	router chi.Router
}

type Comment struct {
	ID        int        `json:"id"`
	NewsID    int        `json:"news_id"`
	ParentID  *int       `json:"parent_id,omitempty"`
	Text      string     `json:"text"`
	CreatedAt time.Time  `json:"created_at"`
}

type Response struct {
	Status     string      `json:"status"`
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

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

func generateRequestID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func LoggerMiddleware(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return hlog.NewHandler(*logger)
}

func NewApp(config Config) *App {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(RequestIDMiddleware)
	r.Use(LoggerMiddleware(&logger))

	app := &App{
		config: config,
		logger: logger,
		router: r,
	}

	var err error
	db, err = sql.Open("sqlite3", config.DBPath)
	if err != nil {
		log.Fatal(err)
	}

	// Создание таблицы
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			news_id INTEGER NOT NULL,
			parent_id INTEGER,
			text TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_news_id ON comments(news_id);
	`)
	if err != nil {
		log.Fatal(err)
	}

	r.Get("/health", app.HealthCheck)
	r.Post("/comments", app.CreateComment)
	r.Get("/comments", app.GetCommentsByNewsID)

	return app
}

func (a *App) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Status: "ok"})
}

func (a *App) CreateComment(w http.ResponseWriter, r *http.Request) {
	var comment Comment
	if err := json.NewDecoder(r.Body).Decode(&comment); err != nil {
		a.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if comment.NewsID < 1 {
		a.sendError(w, http.StatusBadRequest, "Invalid news_id")
		return
	}

	if len(comment.Text) > 1000 {
		a.sendError(w, http.StatusBadRequest, "Text too long")
		return
	}

	if comment.ParentID != nil {
		var exists bool
		err := db.QueryRow("SELECT 1 FROM comments WHERE id = ?", *comment.ParentID).Scan(&exists)
		if err != nil || !exists {
			a.sendError(w, http.StatusBadRequest, "Parent comment does not exist")
			return
		}
	}

	stmt, err := db.Prepare("INSERT INTO comments (news_id, parent_id, text) VALUES (?, ?, ?)")
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer stmt.Close()

	result, err := stmt.Exec(comment.NewsID, comment.ParentID, comment.Text)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to insert comment")
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Failed to get comment ID")
		return
	}

	comment.ID = int(id)
	comment.CreatedAt = time.Now()

	a.sendResponse(w, http.StatusOK, comment)
}

func (a *App) GetCommentsByNewsID(w http.ResponseWriter, r *http.Request) {
	newsIDStr := r.URL.Query().Get("news_id")
	newsID, err := strconv.Atoi(newsIDStr)
	if err != nil || newsID < 1 {
		a.sendError(w, http.StatusBadRequest, "Invalid news_id")
		return
	}

	rows, err := db.Query("SELECT id, news_id, parent_id, text, created_at FROM comments WHERE news_id = ?", newsID)
	if err != nil {
		a.sendError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		var createdAtStr string
		err := rows.Scan(&c.ID, &c.NewsID, &c.ParentID, &c.Text, &createdAtStr)
		if err != nil {
			continue
		}
		c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		comments = append(comments, c)
	}

	a.sendResponse(w, http.StatusOK, comments)
}

func (a *App) sendResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(Response{
		Status: "success",
		Data:   data,
	})
}

func (a *App) sendError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(Response{
		Status: "error",
		Error:  message,
	})
}

func (a *App) Run() error {
	return http.ListenAndServe(":"+a.config.Port, a.router)
}

func main() {
	config := Config{
		Port:   getEnv("PORT", "8081"),
		DBPath: getEnv("DB_PATH", "./comments.db"),
	}

	app := NewApp(config)

	log.Printf("Comment Service запущен на порту %s", config.Port)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}