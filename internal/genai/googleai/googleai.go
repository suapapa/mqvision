// Package googleai implements [genai.VisionClient] using Google GenAI (Gemini) via Genkit and the Files API.
package googleai

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	ggenai "google.golang.org/genai"

	"github.com/suapapa/mqvision/internal/genai"
)

// const geminiModel = "googleai/gemini-2.5-flash-lite"

// Client uploads JPEG input through the GenAI Files API and runs structured generation with Genkit.
type Client struct {
	g *genkit.Genkit
	c *ggenai.Client

	model        string
	systemPrompt string
	promptForImg string
	fixSystem   string
	fixUser     string

	lastRead string
}

// NewClient initializes Genkit with the Google AI plugin and an API-key-backed GenAI HTTP client.
// fixUser may contain {{ambiguous}} and {{previous}} placeholders.
func NewClient(ctx context.Context,
	apiKey string,
	model string,
	systemPrompt string,
	prompt string,
	fixSystem string,
	fixUser string,
) (*Client, error) {
	gk := genkit.Init(ctx, genkit.WithPlugins(&googlegenai.GoogleAI{}))

	// Create Files API client
	c, err := ggenai.NewClient(ctx, &ggenai.ClientConfig{
		Backend: ggenai.BackendGeminiAPI,
		APIKey:  apiKey, // os.Getenv("GEMINI_API_KEY"),
	})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}

	return &Client{
		g:            gk,
		c:            c,
		model:        model,
		systemPrompt: systemPrompt,
		promptForImg: prompt,
		fixSystem:   fixSystem,
		fixUser:     fixUser,
	}, nil
}

// ReadGasGaugePic implements [genai.VisionClient].
func (c *Client) ReadGasGaugePic(
	ctx context.Context,
	jpgReader io.Reader,
) (*genai.GasMeterReadResult, error) {

	start := time.Now()

	// fileSample, err := c.c.Files.UploadFromPath(ctx, "sample/gauge_20251107_051332.jpg", &genai.UploadFileConfig{
	// 	MIMEType:    "image/jpeg",
	// 	DisplayName: "Test Image",
	// })
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to upload: %v", err)
	// }

	// Initialize Genkit
	file, err := c.c.Files.Upload(ctx, jpgReader, &ggenai.UploadFileConfig{
		MIMEType:    "image/jpeg",
		DisplayName: "Gas Meter Image",
	})
	if err != nil {
		return nil, fmt.Errorf("upload image: %w", err)
	}
	// fmt.Printf("Uploaded! File URI: %s\n", file.URI)
	defer func(ctx context.Context, fileName string) {
		// Clean up
		// c.c.Files.Delete(ctx, sampleFileName, nil)
		c.c.Files.Delete(ctx, fileName, nil)
		// fmt.Println("Cleaned up uploaded file")
	}(ctx, file.Name)

	// Use Files API URI directly with Genkit (now supported!)
	// fmt.Println("Analyzing image with Genkit using Files API URI...")

	out, _, err := genkit.GenerateData[genai.GasMeterReadResult](ctx, c.g,
		ai.WithModelName(c.model),
		ai.WithMessages(
			ai.NewSystemMessage(
				// ai.NewMediaPart("image/jpeg", fileSample.URI), // system prompt denies to use image
				// ai.NewTextPart(readGuagePicPrompt),
				ai.NewTextPart(c.systemPrompt),
			),
			ai.NewUserMessage(
				ai.NewMediaPart("image/jpeg", file.URI),
				// ai.NewTextPart("Process the image and extract the reading and date."),
				ai.NewTextPart(c.promptForImg),
			),
		),
		ai.WithConfig(&ggenai.GenerateContentConfig{
			TopK:        float32Ptr(10),
			Temperature: float32Ptr(0.1),
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("analyze image: %w", err)
	}

	if strings.Contains(out.Read, "?") {
		log.Printf("Ambiguous digits found in the reading: %s", out.Read)
		out.Read, err = c.guessAmbiguousDigits(ctx, out.Read)
		if err != nil {
			return nil, fmt.Errorf("guess ambiguous digits: %w", err)
		}
	}

	out.ItTakes = time.Since(start).String()
	out.ReadAt = time.Now()

	c.lastRead = out.Read

	return out, nil
}

// ReadGasGaugePicFromURL downloads the JPEG at imageURL and delegates to ReadGasGaugePic.
func (c *Client) ReadGasGaugePicFromURL(
	ctx context.Context,
	imageURL string,
) (*genai.GasMeterReadResult, error) {
	u := strings.TrimSpace(imageURL)
	if u == "" {
		return nil, fmt.Errorf("empty image URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch image: status %s", resp.Status)
	}
	return c.ReadGasGaugePic(ctx, resp.Body)
}

func (c *Client) guessAmbiguousDigits(
	ctx context.Context,
	ambiguousValueString string,
) (string, error) {
	if !genai.ContainsOnly(ambiguousValueString, ".?0123456789") {
		return "", fmt.Errorf("ambiguous value string %q is not valid", ambiguousValueString)
	}

	userPrompt := strings.ReplaceAll(c.fixUser, "{{ambiguous}}", ambiguousValueString)
	userPrompt = strings.ReplaceAll(userPrompt, "{{previous}}", c.lastRead)

	resp, err := genkit.Generate(ctx, c.g,
		ai.WithModelName(c.model),
		ai.WithMessages(
			ai.NewSystemMessage(
				ai.NewTextPart(c.fixSystem),
			),
			ai.NewUserMessage(
				ai.NewTextPart(userPrompt),
			),
		),
		ai.WithConfig(&ggenai.GenerateContentConfig{
			TopK:        float32Ptr(10),
			Temperature: float32Ptr(0.1),
		}),
	)
	if err != nil {
		return "", fmt.Errorf("generate disambiguation: %w", err)
	}

	return resp.Text(), nil
}

func float32Ptr(v float32) *float32 {
	return &v
}
