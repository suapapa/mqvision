package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRingBuffer(t *testing.T) {
	t.Parallel()

	t.Run("Push and GetAll within capacity", func(t *testing.T) {
		t.Parallel()
		rb := NewRingBuffer(3)
		rb.Push(SensorReading{Value: 1.1})
		rb.Push(SensorReading{Value: 2.2})

		items := rb.GetAll()
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}
		if items[0].Value != 1.1 || items[1].Value != 2.2 {
			t.Errorf("unexpected items: %+v", items)
		}
	})

	t.Run("Push beyond capacity wraps around", func(t *testing.T) {
		t.Parallel()
		rb := NewRingBuffer(3)
		rb.Push(SensorReading{Value: 1.1})
		rb.Push(SensorReading{Value: 2.2})
		rb.Push(SensorReading{Value: 3.3})
		rb.Push(SensorReading{Value: 4.4}) // should overwrite 1.1

		items := rb.GetAll()
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(items))
		}
		if items[0].Value != 2.2 || items[1].Value != 3.3 || items[2].Value != 4.4 {
			t.Errorf("unexpected items: %+v", items)
		}
	})
}

func TestSensorServerHistory(t *testing.T) {
	t.Parallel()

	// Set up server and set some values
	s := &SensorServer{}

	now := time.Now()
	// Set Value internally
	s.SetValue(10.5, "meta1")

	// Artificially modify history timestamps to simulate time passing
	s.Lock()
	// We want one reading within 7 days, one outside 7 days
	s.history.data[0].UpdatedAt = now.Add(-8 * 24 * time.Hour) // Old (filtered out)

	// Add another one (within 7 days)
	s.history.Push(SensorReading{
		Value:     20.5,
		UpdatedAt: now.Add(-3 * 24 * time.Hour), // Within 7 days
		Metadata:  "meta2",
	})
	s.Unlock()

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

	if len(readings) != 1 {
		t.Fatalf("expected 1 reading in response, got %d", len(readings))
	}

	if readings[0].Value != 20.5 {
		t.Errorf("expected value 20.5, got %v", readings[0].Value)
	}
}
