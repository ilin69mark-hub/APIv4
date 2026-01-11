package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

type Config struct {
	Port string
}

type App struct {
	config Config
	logger zerolog.Logger
	router chi.Router
}

type News struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Date    string `json:"date"`
}

type Response struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

var newsList = []News{
	{ID: 1, Title: "Новость 1", Content: "Содержимое первой новости", Date: "2023-01-01"},
	{ID: 2, Title: "Новость 2", Content: "Содержимое второй новости", Date: "2023-01-02"},
	{ID: 3, Title: "Новость 3", Content: "Содержимое третьей новости", Date: "2023-01-03"},
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func LoggerMiddleware(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return hlog.NewHandler(*logger)
}

func NewApp(config Config) *App {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(LoggerMiddleware(&logger))

	app := &App{
		config: config,
		logger: logger,
		router: r,
	}

	r.Get("/", app.Home)
	r.Get("/health", app.HealthCheck)
	r.Get("/news", app.GetNews)
	r.Get("/news/{id}", app.GetNewsByID)

	return app
}

func (a *App) Home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("News Aggregator OK"))
}

func (a *App) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Status: "ok"})
}

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

	var filteredNews []News
	for _, n := range newsList {
		if search == "" || strings.Contains(strings.ToLower(n.Title), strings.ToLower(search)) || strings.Contains(strings.ToLower(n.Content), strings.ToLower(search)) {
			filteredNews = append(filteredNews, n)
		}
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(filteredNews) {
		start = len(filteredNews)
	}
	if end > len(filteredNews) {
		end = len(filteredNews)
	}

	paginatedNews := filteredNews[start:end]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Status: "success",
		Data:   paginatedNews,
	})
}

func (a *App) GetNewsByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id < 1 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Status: "error",
			Error:  "Invalid news ID",
		})
		return
	}

	for _, n := range newsList {
		if n.ID == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Response{
				Status: "success",
				Data:   n,
			})
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(Response{
		Status: "error",
		Error:  "News not found",
	})
}

func (a *App) Run() error {
	return http.ListenAndServe(":"+a.config.Port, a.router)
}

func main() {
	config := Config{
		Port: getEnv("PORT", "8083"),
	}

	app := NewApp(config)

	log.Printf("News Aggregator запущен на порту %s", config.Port)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}