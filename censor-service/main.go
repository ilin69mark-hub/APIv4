package main

import (
	"context"
	"encoding/json"
	"strconv"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

var forbiddenWords = map[string]bool{
	"qwerty": true,
	"йцукен": true,
	"zxvbnm": true,
}

type Config struct {
	Port string
}

type App struct {
	config Config
	logger zerolog.Logger
	router chi.Router
}

type CheckRequest struct {
	Text string `json:"text"`
}

type Response struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
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

	r.Get("/health", app.HealthCheck)
	r.Post("/check", app.CheckText)

	return app
}

func (a *App) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Status: "ok"})
}

func (a *App) CheckText(w http.ResponseWriter, r *http.Request) {
	var req CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	text := strings.ToLower(req.Text)

	for word := range forbiddenWords {
		if strings.Contains(text, strings.ToLower(word)) {
			a.sendError(w, http.StatusBadRequest, "Text contains forbidden words")
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(Response{Status: "success"})
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
		Port: getEnv("PORT", "8082"),
	}

	app := NewApp(config)

	log.Printf("Censor Service запущен на порту %s", config.Port)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}