package main

import (
	"context"
	"flag"
	"fmt"
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
	"github.com/suapapa/mqvision/internal/gemini"
	"github.com/suapapa/mqvision/internal/mqttdump"
)

var (
	flagSingleShot = ""
	flagPort       = "8080"
	flagConfigFile = "config.yaml"

	config *Config

	sensorServer    *SensorServer
	geminiClient    *gemini.Client
	conciergeClient *concierge.Client

	chLuggage chan *Luggage
)

type Luggage struct {
	*gemini.GasMeterReadResult
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
	geminiClient, err = gemini.NewClient(ctx,
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
	defer close(chLuggage)
	go func() {
		for readResult := range chLuggage {
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
	}()

	mqttClient, err := mqttdump.NewClient(config.MQTT.Host, config.MQTT.Topic)
	if err != nil {
		log.Fatalf("Error creating MQTT client: %v", err)
	}
	defer mqttClient.Stop()

	go func() {
		if flagSingleShot != "" {
			imgFileName := flagSingleShot
			log.Printf("Reading image file: %s", imgFileName)
			img, err := os.Open(imgFileName)
			if err != nil {
				log.Fatalf("Error opening image file: %v", err)
			}
			defer img.Close()

			// Create a single Writer and multiple Readers
			pw, prs := SingleInMultiOutPipe(2)
			defer pw.Close()
			defer prs[0].Close()
			defer prs[1].Close()

			// Copy image data to the Writer (broadcasts to all Readers)
			go func() {
				defer pw.Close()
				io.Copy(pw, img)
			}()

			// Use the Readers in parallel
			var wg sync.WaitGroup
			var srcImgStoredURL string
			var readResult *gemini.GasMeterReadResult
			var conciergeErr, geminiErr error

			wg.Add(2)

			// Post to concierge using first reader
			go func() {
				defer wg.Done()
				srcImgStoredURL, conciergeErr = conciergeClient.PostImage(prs[0], "image/jpeg")
				if conciergeErr != nil {
					log.Printf("Error posting image to concierge: %v", conciergeErr)
				} else {
					log.Printf("Posted image to concierge: %s", srcImgStoredURL)
				}
			}()

			// Read gauge using second reader
			go func() {
				defer wg.Done()
				readResult, geminiErr = geminiClient.ReadGasGuagePic(context.Background(), prs[1])
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
	router := gin.Default()
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

	// Gracefully shutdown the server with a timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

func mqttReadGuageSubHandler() io.WriteCloser {
	prGemini, pwGemini := io.Pipe()
	prConcierge, pwConcierge := io.Pipe()

	// Create a MultiWriter that writes to both pipes
	mw := io.MultiWriter(pwGemini, pwConcierge)

	// Create a WriteCloser that closes both pipes when closed
	pw := &multiWriteCloser{
		Writer:  mw,
		closers: []io.Closer{pwGemini, pwConcierge},
	}

	go func() {
		defer prConcierge.Close()
		defer prGemini.Close()

		srcImgStoredURL, err := conciergeClient.PostImage(prConcierge, "image/jpeg")
		if err != nil {
			log.Printf("Error posting image to concierge: %v", err)
			return
		}
		log.Printf("Posted image to concierge: %s", srcImgStoredURL)

		readResult, err := geminiClient.ReadGasGuagePic(context.Background(), prGemini)
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

// multiWriteCloser wraps io.MultiWriter to implement io.WriteCloser
type multiWriteCloser struct {
	io.Writer
	closers []io.Closer
}

func (m *multiWriteCloser) Close() error {
	var errs []error
	for _, closer := range m.closers {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing writers: %v", errs)
	}
	return nil
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
