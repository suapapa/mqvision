// Package openaicompat implements genai.VisionClient against an OpenAI-compatible chat/completions API.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/suapapa/mqvision/internal/genai"
)

// Client calls an OpenAI-compatible HTTP API for vision + structured JSON extraction.
type Client struct {
	httpClient   *http.Client
	baseURL      string
	apiKey       string
	model        string
	systemPrompt string
	promptForImg string
	fixSystem   string
	fixUser     string
	lastRead     string
}

// NewClient constructs a Client. baseURL should be the API root (e.g. https://host/v1) without a trailing slash.
// fixUser may contain {{ambiguous}} and {{previous}} placeholders.
func NewClient(
	baseURL, apiKey, model, systemPrompt, promptForImg, fixSystem, fixUser string,
) *Client {
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &Client{
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		baseURL:      b,
		apiKey:       apiKey,
		model:        model,
		systemPrompt: systemPrompt,
		promptForImg: promptForImg,
		fixSystem:   fixSystem,
		fixUser:     fixUser,
	}
}

// ReadGasGaugePicFromURL runs the same analysis as ReadGasGaugePic using a public image URL.
// imageURL must be reachable by the API provider (typically https).
func (c *Client) ReadGasGaugePicFromURL(
	ctx context.Context,
	imageURL string,
) (*genai.GasMeterReadResult, error) {
	u := strings.TrimSpace(imageURL)
	if u == "" {
		return nil, fmt.Errorf("empty image URL")
	}
	return c.readGasGaugeFromVisionURL(ctx, u)
}

// ReadGasGaugePic implements [genai.VisionClient].
func (c *Client) ReadGasGaugePic(
	ctx context.Context,
	jpgReader io.Reader,
) (*genai.GasMeterReadResult, error) {
	jpgBytes, err := io.ReadAll(jpgReader)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	if len(jpgBytes) == 0 {
		return nil, fmt.Errorf("empty image")
	}
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpgBytes)
	return c.readGasGaugeFromVisionURL(ctx, dataURL)
}

// readGasGaugeFromVisionURL sends imageURL as an OpenAI-style image_url (data URI or https URL).
func (c *Client) readGasGaugeFromVisionURL(ctx context.Context, imageURL string) (*genai.GasMeterReadResult, error) {
	start := time.Now()

	content, err := c.chatCompletion(ctx, []chatMessage{
		{Role: "system", Content: c.systemPrompt},
		{Role: "user", Content: []contentPart{
			{Type: "text", Text: c.promptForImg},
			{Type: "image_url", ImageURL: &imageURLPart{URL: imageURL}},
		}},
	}, 0.1)
	if err != nil {
		return nil, err
	}

	out, err := parseGasMeterJSON(content)
	if err != nil {
		return nil, fmt.Errorf("parse model JSON: %w", err)
	}

	out.Read = genai.NormalizeReading(out.Read)

	if strings.Contains(out.Read, "?") {
		log.Printf("Ambiguous digits found in the reading: %s", out.Read)
		fixed, err := c.guessAmbiguousDigits(ctx, out.Read)
		if err != nil {
			return nil, fmt.Errorf("guess ambiguous digits: %w", err)
		}
		out.Read = genai.NormalizeReading(fixed)
	}

	out.ItTakes = time.Since(start).String()
	out.ReadAt = time.Now()
	c.lastRead = out.Read
	return out, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type contentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *imageURLPart `json:"image_url,omitempty"`
}

type imageURLPart struct {
	URL string `json:"url"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) chatCompletion(ctx context.Context, messages []chatMessage, temperature float64) (string, error) {
	body := chatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: temperature,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var parsed chatCompletionResponse
	decodeErr := json.Unmarshal(respBody, &parsed)
	if decodeErr != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("http status %d: %w; body: %s", resp.StatusCode, decodeErr, truncate(string(respBody), 500))
		}
		return "", fmt.Errorf("decode response (status %d): %w; body: %s", resp.StatusCode, decodeErr, truncate(string(respBody), 500))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("api error: %s", parsed.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http status %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("no choices in response: %s", truncate(string(respBody), 500))
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("empty message content")
	}
	return content, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func parseGasMeterJSON(text string) (*genai.GasMeterReadResult, error) {
	jsonStr := extractJSONObject(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON object in model output: %s", truncate(text, 300))
	}
	var out genai.GasMeterReadResult
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	return &out, nil
}

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	s = stripMarkdownFence(s)

	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
		if idx := strings.Index(s, "\n"); idx != -1 {
			// optional language tag on first line
			rest := s[idx+1:]
			if end := strings.Index(rest, "```"); end != -1 {
				return strings.TrimSpace(rest[:end])
			}
			return strings.TrimSpace(rest)
		}
	}
	return s
}

func (c *Client) guessAmbiguousDigits(ctx context.Context, ambiguousValueString string) (string, error) {
	if !genai.ContainsOnly(ambiguousValueString, ".?0123456789") {
		return "", fmt.Errorf("ambiguous value string %q is not valid", ambiguousValueString)
	}
	userPrompt := strings.ReplaceAll(c.fixUser, "{{ambiguous}}", ambiguousValueString)
	userPrompt = strings.ReplaceAll(userPrompt, "{{previous}}", c.lastRead)
	content, err := c.chatCompletion(ctx, []chatMessage{
		{Role: "system", Content: c.fixSystem},
		{Role: "user", Content: userPrompt},
	}, 0.1)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(content), nil
}
