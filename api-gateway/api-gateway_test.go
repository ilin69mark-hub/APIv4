package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	req, _ := http.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	config := Config{Port: "8080"}
	app := NewApp(config)
	app.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Ожидался статус %d, получен %d", http.StatusOK, rr.Code)
	}
}