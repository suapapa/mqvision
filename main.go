package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/suapapa/mqvision/internal/concierge"
	"github.com/suapapa/mqvision/internal/genai"
	"github.com/suapapa/mqvision/internal/genai/openaicompat"
	"github.com/suapapa/mqvision/internal/mqttdump"
	// "github.com/suapapa/mqvision/internal/genai/googleai"
)

const mqttDisconnectExitAfter = 2 * time.Minute

var (
	flagSingleShot = ""
	flagPort       = "8080"
	flagConfigFile = "prompt.yaml"

	config *Config

	sensorServer    *SensorServer
	genaiClient     genai.VisionClient
	conciergeClient *concierge.Client
	mqttClient      *mqttdump.Client

	chLuggage chan *Luggage

	// appCtx is the process-wide context for downstream API calls (cancelled on shutdown).
	appCtx context.Context
)

// watchMQTTDisconnect closes exitCh if MQTT stays disconnected for longer than threshold.
func watchMQTTDisconnect(ctx context.Context, threshold time.Duration, exitCh chan<- struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var disconnectedSince time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if mqttClient == nil {
				continue
			}
			connected, _ := mqttClient.Status()
			if connected {
				disconnectedSince = time.Time{}
				continue
			}
			if disconnectedSince.IsZero() {
				disconnectedSince = time.Now()
				continue
			}
			if time.Since(disconnectedSince) >= threshold {
				close(exitCh)
				return
			}
		}
	}
}

func newVisionClient(ctx context.Context, c *Config) (genai.VisionClient, error) {
	base := strings.TrimSpace(c.OpenAICompat.BaseURL)
	key := strings.TrimSpace(c.OpenAICompat.APIKey)
	if base == "" || key == "" {
		return nil, fmt.Errorf("configure OPENAI_BASE_URL and OPENAI_API_KEY")
	}
	log.Println("Creating OpenAI-compatible vision client")
	return openaicompat.NewClient(
		c.OpenAICompat.BaseURL,
		c.OpenAICompat.APIKey,
		c.OpenAICompat.Model,
		c.ReadGasGauge.System,
		c.ReadGasGauge.User,
		c.FixAmbiguous.System,
		c.FixAmbiguous.User,
	), nil
	// if strings.TrimSpace(c.Gemini.APIKey) == "" {
	// 	return nil, fmt.Errorf("configure openai_compat (base_url + api_key) or gemini (api_key)")
	// }
	// log.Println("Creating Gemini client")
	// return googleai.NewClient(ctx,
	// 	c.Gemini.APIKey,
	// 	c.Gemini.Model,
	// 	c.ReadGasGauge.System,
	// 	c.ReadGasGauge.User,
	// 	c.FixAmbiguous.System,
	// 	c.FixAmbiguous.User,
	// )
}

type Luggage struct {
	*genai.GasMeterReadResult
	SrcImageURL string `json:"src_image_url"`
}

func main() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	appCtx = ctx

	flag.StringVar(&flagPort, "p", "8080", "Port to listen on")
	flag.StringVar(&flagSingleShot, "i", "", "Single run on a image file (testing purpose)")
	flag.StringVar(&flagConfigFile, "c", "prompt.yaml", "Prompt config file to use")
	flag.Parse()

	config, err = LoadConfig(flagConfigFile)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	genaiClient, err = newVisionClient(ctx, config)
	if err != nil {
		log.Fatalf("Error creating vision client: %v", err)
	}

	log.Println("Creating concierge client")
	conciergeClient = concierge.NewClient(config.Concierge.Addr, config.Concierge.Token)

	log.Println("Creating sensor server")
	sensorServer = &SensorServer{}

	chLuggage = make(chan *Luggage, 10)
	var wg sync.WaitGroup
	wg.Add(1)
	go func(ctx context.Context) {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case readResult, ok := <-chLuggage:
				if !ok {
					return
				}
				// jsonBytes, err := json.MarshalIndent(readResult, "", "  ")
				// if err != nil {
				// 	log.Printf("Error marshalling read result: %v", err)
				// 	continue
				// }
				// os.Stdout.Write(jsonBytes)
				// os.Stdout.WriteString("\n")

				read, err := strconv.ParseFloat(readResult.Read, 64)
				if err != nil {
					log.Printf("Error parsing read value: %v", err)
					continue
				}

				sensorServer.SetValue(read, readResult)
				log.Printf("Updated sensor value: %s (%.3f)", readResult.Read, read)
			}
		}
	}(ctx)

	mqttClient, err = mqttdump.NewClient(config.MQTT.Host, config.MQTT.Topic)
	if err != nil {
		log.Fatalf("Error creating MQTT client: %v", err)
	}
	defer mqttClient.Stop()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if flagSingleShot != "" {
			imgFileName := flagSingleShot
			log.Printf("Reading image file: %s", imgFileName)
			img, err := os.Open(imgFileName)
			if err != nil {
				log.Fatalf("Error opening image file: %v", err)
			}
			defer img.Close()

			imgBytes, err := io.ReadAll(img)
			if err != nil {
				log.Fatalf("Error reading image file: %v", err)
			}

			var srcImgStoredURL string
			var readResult *genai.GasMeterReadResult

			if strings.TrimSpace(config.Concierge.Addr) != "" && strings.TrimSpace(config.Concierge.Token) != "" {
				srcImgStoredURL, err = conciergeClient.PostImage(bytes.NewReader(imgBytes), "image/jpeg")
				if err != nil {
					log.Printf("Error posting image to concierge: %v", err)
				} else {
					log.Printf("Posted image to concierge: %s", srcImgStoredURL)
				}
			}

			if srcImgStoredURL != "" {
				readResult, err = genaiClient.ReadGasGaugePicFromURL(appCtx, srcImgStoredURL)
			} else {
				readResult, err = genaiClient.ReadGasGaugePic(appCtx, bytes.NewReader(imgBytes))
			}
			if err != nil {
				log.Printf("Error reading gauge image: %v", err)
				return
			}

			if readResult == nil {
				log.Printf("Read result is nil for image: %s", imgFileName)
				return
			}

			l := &Luggage{
				GasMeterReadResult: readResult,
				SrcImageURL:        srcImgStoredURL,
			}
			chLuggage <- l
		} else {
			log.Println("Running MQTT client")

			hdlr := mqttReadGaugeSubHandler
			if err := mqttClient.Run(hdlr); err != nil {
				log.Fatalf("Error running MQTT client: %v", err)
			}

			log.Println("MQTT client running")
		}
	}()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// gin.SetMode(gin.ReleaseMode)
	log.Printf("Starting Gin server on port %s", flagPort)
	router := gin.New()
	router.Use(gin.Recovery())
	// router.Use(gin.Logger())
	router.GET("/api/sensor", sensorServer.GetValueHandler)
	router.GET("/api/sensors", sensorServer.GetHistoryHandler)
	router.GET("/api/health", healthHandler)
	mountWebUI(router, "web/dist")

	// Create HTTP server with graceful shutdown support
	srv := &http.Server{
		Addr:    ":" + flagPort,
		Handler: router,
	}

	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error running Gin server: %v", err)
		}
	}()

	log.Println("Server started. Press Ctrl+C to stop.")

	// If MQTT stays down long enough, exit so Docker restart:unless-stopped can recover.
	watchdogExit := make(chan struct{})
	if flagSingleShot == "" {
		go watchMQTTDisconnect(ctx, mqttDisconnectExitAfter, watchdogExit)
	}

	exitCode := 0
	select {
	case <-sigChan:
		log.Println("Shutting down server...")
	case <-watchdogExit:
		log.Printf("MQTT disconnected for %v; exiting for container restart", mqttDisconnectExitAfter)
		exitCode = 1
	}

	// Cancel context to signal all goroutines
	cancel()

	// Stop MQTT client first
	if mqttClient != nil {
		log.Println("Stopping MQTT client...")
		if err := mqttClient.Stop(); err != nil {
			log.Printf("Error stopping MQTT client: %v", err)
		}
	}

	// Close chLuggage channel to signal the goroutine to exit
	close(chLuggage)

	// Gracefully shutdown the server with a timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Wait for all goroutines to finish
	log.Println("Waiting for goroutines to finish...")
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All goroutines finished")
	case <-time.After(5 * time.Second):
		log.Println("Timeout waiting for goroutines to finish")
	}

	log.Println("Server stopped")
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func mqttReadGaugeSubHandler() io.WriteCloser {
	pr, pw := io.Pipe()

	go func() {
		defer pr.Close()

		imgBytes, err := io.ReadAll(pr)
		if err != nil {
			log.Printf("Error reading MQTT image stream: %v", err)
			return
		}

		var srcImgStoredURL string
		var readResult *genai.GasMeterReadResult

		if strings.TrimSpace(config.Concierge.Addr) != "" && strings.TrimSpace(config.Concierge.Token) != "" {
			srcImgStoredURL, err = conciergeClient.PostImage(bytes.NewReader(imgBytes), "image/jpeg")
			if err != nil {
				log.Printf("Error posting image to concierge: %v", err)
			} else {
				log.Printf("Posted image to concierge: %s", srcImgStoredURL)
			}
		}

		if srcImgStoredURL != "" {
			readResult, err = genaiClient.ReadGasGaugePicFromURL(appCtx, srcImgStoredURL)
		} else {
			readResult, err = genaiClient.ReadGasGaugePic(appCtx, bytes.NewReader(imgBytes))
		}
		if err != nil {
			log.Printf("Error reading gauge image: %v", err)
			return
		}
		if readResult == nil {
			log.Printf("Read result is nil")
			return
		}
		log.Printf("Read result: %+v", readResult)

		l := &Luggage{
			GasMeterReadResult: readResult,
			SrcImageURL:        srcImgStoredURL,
		}
		chLuggage <- l
	}()

	return pw
}

func healthHandler(c *gin.Context) {
	if mqttClient == nil {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"info":   "single-shot mode or mqtt client not initialized",
		})
		return
	}

	isConnected, lastErr := mqttClient.Status()

	status := "ok"
	httpStatus := http.StatusOK
	var errMsg *string
	if lastErr != nil {
		s := lastErr.Error()
		errMsg = &s
	}

	if !isConnected {
		if flagSingleShot == "" {
			status = "fail"
			httpStatus = http.StatusServiceUnavailable
		}
	}

	sensorServer.RLock()
	lastUpdated := sensorServer.UpdatedAt
	sensorServer.RUnlock()

	var lastUpdatedStr *string
	if !lastUpdated.IsZero() {
		s := lastUpdated.Format(time.RFC3339)
		lastUpdatedStr = &s
	}

	response := gin.H{
		"status": status,
		"mqtt": gin.H{
			"connected":  isConnected,
			"last_error": errMsg,
		},
		"sensor": gin.H{
			"last_updated": lastUpdatedStr,
		},
	}

	c.JSON(httpStatus, response)
}

// func mqttFileDumpSubHandler() io.WriteCloser {
// 	timestamp := time.Now().Format("20060102_150405")
// 	filename := fmt.Sprintf("gauge_%s.jpg", timestamp)

// 	// Create output directory if it doesn't exist
// 	outputDir := "images"
// 	if err := os.MkdirAll(outputDir, 0755); err != nil {
// 		log.Printf("Error creating directory: %v", err)
// 		return nil
// 	}

// 	filepath := filepath.Join(outputDir, filename)

// 	f, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0644)
// 	if err != nil {
// 		log.Printf("Error opening file: %v", err)
// 		return nil
// 	}
// 	return f
// }
