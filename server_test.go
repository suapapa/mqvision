package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSensorServerHistory(t *testing.T) {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}
	dbName := "mqvision_test"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	s, err := NewSensorServer(ctx, mongoURI, dbName)
	if err != nil {
		t.Skipf("Skipping MongoDB test: connection failed: %v", err)
	}
	defer func() {
		// clean up test database
		_ = s.db.Drop(ctx)
		_ = s.Close(ctx)
	}()

	now := time.Now()

	// Insert test data using SetValue
	err = s.SetValue(ctx, 10.5, "meta1")
	if err != nil {
		t.Fatalf("failed to set value: %v", err)
	}

	// We want to insert another document, but since SetValue automatically sets updated_at to now,
	// if we want to test filtering, we can directly insert a document with past timestamp to mongodb
	pastReading := SensorReading{
		Value:     20.5,
		UpdatedAt: now.Add(-8 * 24 * time.Hour), // Old (should be filtered out since it's > 7 days)
		Metadata:  "meta2",
	}
	_, err = s.collection.InsertOne(ctx, pastReading)
	if err != nil {
		t.Fatalf("failed to insert past reading: %v", err)
	}

	// Add another one within 7 days
	recentReading := SensorReading{
		Value:     30.5,
		UpdatedAt: now.Add(-3 * 24 * time.Hour), // Within 7 days
		Metadata:  "meta3",
	}
	_, err = s.collection.InsertOne(ctx, recentReading)
	if err != nil {
		t.Fatalf("failed to insert recent reading: %v", err)
	}

	// Test the Gin handler
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/sensors", s.GetHistoryHandler)

	req, err := http.NewRequest(http.MethodGet, "/api/sensors", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %d", w.Code)
	}

	var readings []SensorReading
	if err := json.Unmarshal(w.Body.Bytes(), &readings); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// We expect 2 readings:
	// 1. 10.5 (set just now)
	// 2. 30.5 (set at now - 3 days)
	// The 20.5 (at now - 8 days) should be filtered out.
	if len(readings) != 2 {
		t.Fatalf("expected 2 readings in response, got %d", len(readings))
	}

	// Sorted by updated_at ascending.
	// 30.5 is older than 10.5, so index 0 should be 30.5, index 1 should be 10.5
	if readings[0].Value != 30.5 || readings[1].Value != 10.5 {
		t.Errorf("unexpected readings order or values: %+v", readings)
	}
}

func TestMountWebUI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	index := filepath.Join(dir, "index.html")
	if err := os.WriteFile(index, []byte("<!doctype html><title>mqvision</title>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	assets := filepath.Join(dir, "assets")
	if err := os.Mkdir(assets, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assets, "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	mountWebUI(router, dir)

	t.Run("serves index", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "mqvision") {
			t.Fatalf("unexpected body: %s", w.Body.String())
		}
	})

	t.Run("spa fallback", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/dashboard", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "mqvision") {
			t.Fatalf("unexpected body: %s", w.Body.String())
		}
	})

	t.Run("api 404 stays json", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/missing", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("serves assets", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/assets/app.js", nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
	})
}

