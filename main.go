package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/suapapa/mqvision/internal/concierge"
	"github.com/suapapa/mqvision/internal/genai"
	"github.com/suapapa/mqvision/internal/mqttdump"
)

var (
	flagSingleShot = ""
	flagPort       = "8080"
	flagConfigFile = "config.yaml"

	config *Config

	sensorServer    *SensorServer
	genaiClient     *genai.Client
	conciergeClient *concierge.Client

	chLuggage chan *Luggage
)

type Luggage struct {
	*genai.GasMeterReadResult
	SrcImageURL string `json:"src_image_url"`
}

func main() {
	var err error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flag.StringVar(&flagPort, "p", "8080", "Port to listen on")
	flag.StringVar(&flagSingleShot, "i", "", "Single run on a image file (testing purpose)")
	flag.StringVar(&flagConfigFile, "c", "config.yaml", "Config file to use")
	flag.Parse()

	config, err = LoadConfig(flagConfigFile)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	log.Println("Creating Gemini client")
	genaiClient, err = genai.NewClient(ctx,
		config.Gemini.APIKey,
		config.Gemini.Model,
		config.Gemini.SystemPrompt, config.Gemini.Prompt,
	)
	if err != nil {
		log.Fatalf("Error creating Gemini client: %v", err)
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

	mqttClient, err := mqttdump.NewClient(config.MQTT.Host, config.MQTT.Topic)
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

			// Create a single Writer and multiple Readers
			pIn, pOuts := SingleInMultiOutPipe(2)
			defer pIn.Close()
			defer pOuts[0].Close()
			defer pOuts[1].Close()

			// Copy image data to the Writer (broadcasts to all Readers)
			go func() {
				defer pIn.Close()
				io.Copy(pIn, img)
			}()

			// Use the Readers in parallel
			var wg sync.WaitGroup
			var srcImgStoredURL string
			var readResult *genai.GasMeterReadResult
			var conciergeErr, geminiErr error

			wg.Add(2)

			// Post to concierge using first reader
			go func() {
				defer wg.Done()
				srcImgStoredURL, conciergeErr = conciergeClient.PostImage(pOuts[0], "image/jpeg")
				if conciergeErr != nil {
					log.Printf("Error posting image to concierge: %v", conciergeErr)
				} else {
					log.Printf("Posted image to concierge: %s", srcImgStoredURL)
				}
			}()

			// Read gauge using second reader
			go func() {
				defer wg.Done()
				readResult, geminiErr = genaiClient.ReadGasGuagePic(context.Background(), pOuts[1])
				if geminiErr != nil {
					log.Printf("Error reading gauge image: %v", geminiErr)
				}
			}()

			wg.Wait()

			if geminiErr != nil {
				log.Printf("Error reading gauge image: %v", geminiErr)
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

			hdlr := mqttReadGuageSubHandler
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
	router.GET("/sensor", sensorServer.GetValueHandler)

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

	// Wait for interrupt signal
	<-sigChan
	log.Println("Shutting down server...")

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
}

func mqttReadGuageSubHandler() io.WriteCloser {
	pIn, pOuts := SingleInMultiOutPipe(2)

	go func() {
		defer pOuts[0].Close()
		defer pOuts[1].Close()

		var wg sync.WaitGroup
		wg.Add(2)

		var srcImgStoredURL string
		var readResult *genai.GasMeterReadResult

		go func() {
			defer wg.Done()

			var err error
			srcImgStoredURL, err = conciergeClient.PostImage(pOuts[0], "image/jpeg")
			if err != nil {
				log.Printf("Error posting image to concierge: %v", err)
				return
			}
			log.Printf("Posted image to concierge: %s", srcImgStoredURL)
		}()

		go func() {
			defer wg.Done()

			var err error
			readResult, err = genaiClient.ReadGasGuagePic(context.Background(), pOuts[1])
			if err != nil {
				log.Printf("Error reading gauge image: %v", err)
				return
			}
			if readResult == nil {
				log.Printf("Read result is nil")
				return
			}
			log.Printf("Read result: %+v", readResult)
		}()

		wg.Wait()
		l := &Luggage{
			GasMeterReadResult: readResult,
			SrcImageURL:        srcImgStoredURL,
		}
		chLuggage <- l
	}()

	return pIn
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
