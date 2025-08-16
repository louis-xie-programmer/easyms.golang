package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestApiProxyNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.Any("/api/:service/*action", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/unknown/hello", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}
