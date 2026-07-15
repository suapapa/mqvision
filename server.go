package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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
	Value     float64   `json:"value" bson:"value"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
	Metadata  any       `json:"metadata" bson:"metadata"`
}

type SensorServer struct {
	Value     float64   `json:"value"`      // latest value
	UpdatedAt time.Time `json:"updated_at"` // latest updated at
	Metadata  any       `json:"metadata"`   // latest metadata

	client     *mongo.Client
	db         *mongo.Database
	collection *mongo.Collection

	sync.RWMutex
}

// NewSensorServer initializes the MongoDB connection, creates a Time Series collection if not exists,
// and loads the latest reading to initialize the in-memory cache.
func NewSensorServer(ctx context.Context, uri, dbName string) (*SensorServer, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("mongo ping: %w", err)
	}

	db := client.Database(dbName)
	collName := "sensor_readings"

	// Check if collection exists
	names, err := db.ListCollectionNames(ctx, bson.M{"name": collName})
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}

	if len(names) == 0 {
		// Create Time Series collection
		opts := options.CreateCollection().SetTimeSeriesOptions(
			options.TimeSeries().
				SetTimeField("updated_at").
				SetMetaField("metadata").
				SetGranularity("minutes"),
		)
		if err := db.CreateCollection(ctx, collName, opts); err != nil {
			return nil, fmt.Errorf("create timeseries collection: %w", err)
		}
	}

	coll := db.Collection(collName)
	s := &SensorServer{
		client:     client,
		db:         db,
		collection: coll,
	}

	// Initialize in-memory cache with the latest document
	var latest SensorReading
	findOpts := options.FindOne().SetSort(bson.M{"updated_at": -1})
	err = coll.FindOne(ctx, bson.M{}, findOpts).Decode(&latest)
	if err == nil {
		s.Value = latest.Value
		s.UpdatedAt = latest.UpdatedAt
		s.Metadata = normalizeMetadata(latest.Metadata)
	} else if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("load latest reading: %w", err)
	}

	return s, nil
}

// Close closes the MongoDB connection.
func (s *SensorServer) Close(ctx context.Context) error {
	if s.client != nil {
		return s.client.Disconnect(ctx)
	}
	return nil
}

// SetValue stores the reading into MongoDB timeseries collection and updates the in-memory cache.
func (s *SensorServer) SetValue(ctx context.Context, value float64, metadata any) error {
	s.Lock()
	defer s.Unlock()

	now := time.Now()
	reading := SensorReading{
		Value:     value,
		UpdatedAt: now,
		Metadata:  metadata,
	}

	_, err := s.collection.InsertOne(ctx, reading)
	if err != nil {
		return fmt.Errorf("insert reading: %w", err)
	}

	s.Value = value
	s.Metadata = metadata
	s.UpdatedAt = now

	return nil
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
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	filter := bson.M{
		"updated_at": bson.M{
			"$gte": cutoff,
		},
	}
	findOpts := options.Find().SetSort(bson.M{"updated_at": 1})

	cursor, err := s.collection.Find(c.Request.Context(), filter, findOpts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to fetch history: %v", err),
		})
		return
	}
	defer cursor.Close(c.Request.Context())

	var readings []SensorReading = []SensorReading{}
	if err := cursor.All(c.Request.Context(), &readings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to decode history: %v", err),
		})
		return
	}

	for i := range readings {
		readings[i].Metadata = normalizeMetadata(readings[i].Metadata)
	}

	c.JSON(http.StatusOK, readings)
}

func normalizeMetadata(metadata any) any {
	if metadata == nil {
		return nil
	}

	var m map[string]any

	switch v := metadata.(type) {
	case map[string]any:
		m = v
	case bson.M:
		m = map[string]any(v)
	case bson.D:
		m = make(map[string]any)
		for _, elem := range v {
			m[elem.Key] = elem.Value
		}
	default:
		return metadata
	}

	// 1. If "gasmeterreadresult" key exists (nested map or bson.D/bson.M), flatten it.
	if val, ok := m["gasmeterreadresult"]; ok {
		var subMap map[string]any
		switch subVal := val.(type) {
		case map[string]any:
			subMap = subVal
		case bson.M:
			subMap = map[string]any(subVal)
		case bson.D:
			subMap = make(map[string]any)
			for _, elem := range subVal {
				subMap[elem.Key] = elem.Value
			}
		}
		if subMap != nil {
			for k, v := range subMap {
				if _, exists := m[k]; !exists {
					m[k] = v
				}
			}
		}
		delete(m, "gasmeterreadresult")
	}

	// 2. Map old field names to new ones if new ones don't exist
	fieldMappings := map[string]string{
		"srcimageurl": "src_image_url",
		"readat":      "read_at",
		"ittakes":     "it_takes",
	}

	for oldKey, newKey := range fieldMappings {
		if val, ok := m[oldKey]; ok {
			if _, exists := m[newKey]; !exists {
				m[newKey] = val
			}
			delete(m, oldKey)
		}
	}

	return m
}
