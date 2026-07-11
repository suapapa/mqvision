package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// mountWebUI serves the Vite-built SPA from webRoot (typically web/dist).
// API routes under /api keep their own 404; everything else falls back to index.html.
func mountWebUI(router *gin.Engine, webRoot string) {
	info, err := os.Stat(webRoot)
	if err != nil || !info.IsDir() {
		return
	}

	assets := filepath.Join(webRoot, "assets")
	if st, err := os.Stat(assets); err == nil && st.IsDir() {
		router.Static("/assets", assets)
	}

	for _, name := range []string{"favicon.svg", "icons.svg", "og-image.jpg"} {
		p := filepath.Join(webRoot, name)
		if _, err := os.Stat(p); err == nil {
			router.StaticFile("/"+name, p)
		}
	}

	index := filepath.Join(webRoot, "index.html")
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.File(index)
	})
}

type SensorReading struct {
	Value     float64   `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
	Metadata  any       `json:"metadata"`
}

type RingBuffer struct {
	data     []SensorReading
	capacity int
	start    int
	size     int
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data:     make([]SensorReading, capacity),
		capacity: capacity,
	}
}

func (r *RingBuffer) Push(item SensorReading) {
	if r.size < r.capacity {
		index := (r.start + r.size) % r.capacity
		r.data[index] = item
		r.size++
	} else {
		r.data[r.start] = item
		r.start = (r.start + 1) % r.capacity
	}
}

func (r *RingBuffer) GetAll() []SensorReading {
	items := make([]SensorReading, 0, r.size)
	for i := 0; i < r.size; i++ {
		index := (r.start + i) % r.capacity
		items = append(items, r.data[index])
	}
	return items
}

type SensorServer struct {
	Value     float64   `json:"value"`      // latest value
	UpdatedAt time.Time `json:"updated_at"` // latest updated at
	Metadata  any       `json:"metadata"`   // latest metadata

	history *RingBuffer

	sync.RWMutex
}

func (s *SensorServer) SetValue(value float64, metadata any) {
	s.Lock()
	defer s.Unlock()
	s.Value = value
	s.Metadata = metadata
	s.UpdatedAt = time.Now()

	if s.history == nil {
		s.history = NewRingBuffer(10080) // Capacity for 7 days of 1-minute updates
	}
	s.history.Push(SensorReading{
		Value:     value,
		UpdatedAt: s.UpdatedAt,
		Metadata:  metadata,
	})
}

func (s *SensorServer) GetValueHandler(c *gin.Context) {
	s.RLock()
	defer s.RUnlock()

	if s.UpdatedAt.IsZero() {
		c.JSON(http.StatusTooEarly, gin.H{
			"error": "no value yet",
		})
		return
	}

	c.JSON(http.StatusOK, s)
}

func (s *SensorServer) GetHistoryHandler(c *gin.Context) {
	s.RLock()
	defer s.RUnlock()

	if s.history == nil {
		c.JSON(http.StatusOK, []SensorReading{})
		return
	}

	allReadings := s.history.GetAll()
	cutoff := time.Now().Add(-7 * 24 * time.Hour)

	var filtered []SensorReading
	for _, r := range allReadings {
		if r.UpdatedAt.After(cutoff) {
			filtered = append(filtered, r)
		}
	}

	c.JSON(http.StatusOK, filtered)
}
