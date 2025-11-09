// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

// This sample shows how to upload a file to Gemini Files API and use it directly with Genkit.
//
// Usage:
//   1. Set GEMINI_API_KEY environment variable
//   2. Run: go run main.go

package gemini

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"google.golang.org/genai"
)

// const geminiModel = "googleai/gemini-2.5-flash-lite"

func float32Ptr(v float32) *float32 {
	return &v
}

type Client struct {
	g *genkit.Genkit
	c *genai.Client

	model        string
	systemPrompt string
	prompt       string
}

func NewClient(ctx context.Context,
	apiKey string,
	model string,
	systemPrompt string,
	prompt string,
) (*Client, error) {
	gk := genkit.Init(ctx, genkit.WithPlugins(&googlegenai.GoogleAI{}))

	// Create Files API client
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
		APIKey:  apiKey, // os.Getenv("GEMINI_API_KEY"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return &Client{
		g:            gk,
		c:            c,
		model:        model,
		systemPrompt: systemPrompt,
		prompt:       prompt,
	}, nil
}

func (c *Client) ReadGasGuagePic(
	ctx context.Context,
	jpgReader io.Reader,
) (*GasMeterReadResult, error) {

	start := time.Now()

	// fileSample, err := c.c.Files.UploadFromPath(ctx, "sample/gauge_20251107_051332.jpg", &genai.UploadFileConfig{
	// 	MIMEType:    "image/jpeg",
	// 	DisplayName: "Test Image",
	// })
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to upload: %v", err)
	// }

	// Initialize Genkit
	file, err := c.c.Files.Upload(ctx, jpgReader, &genai.UploadFileConfig{
		MIMEType:    "image/jpeg",
		DisplayName: "Gas Meter Image",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload: %v", err)
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

	out, _, err := genkit.GenerateData[GasMeterReadResult](ctx, c.g,
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
				ai.NewTextPart(c.prompt),
			),
		),
		ai.WithConfig(&genai.GenerateContentConfig{
			TopK:        float32Ptr(10),
			Temperature: float32Ptr(0.1),
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze: %v", err)
	}

	out.ItTakes = time.Since(start).String()
	out.ReadAt = time.Now()
	return out, nil
}

func (c *Client) ParseAmbiguousDigits(
	ctx context.Context,
	previousValue float64,
	ambiguousValueString string,
) (string, error) {

	// check if ambigousVauleString only has ? characters and digits characters
	if !containsOnly(ambiguousValueString, ".?0123456789") {
		return "", fmt.Errorf("ambious value string, %s is not valid", ambiguousValueString)
	}

	resp, err := genkit.Generate(ctx, c.g,
		ai.WithModelName(c.model),
		ai.WithMessages(
			ai.NewUserMessage(
				ai.NewTextPart(fmt.Sprintf(fixAmbiguousPromptFmt, ambiguousValueString, previousValue)),
			),
		),
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate: %v", err)
	}

	return resp.Text(), nil
}

type GasMeterReadResult struct {
	Read    string    `json:"read"`
	Date    string    `json:"date"`
	ReadAt  time.Time `json:"read_at,omitempty"`
	ItTakes string    `json:"it_takes,omitempty"`
}

func containsOnly(s string, chars string) bool {
	for _, c := range s {
		if !strings.Contains(chars, string(c)) {
			return false
		}
	}
	return true
}

const fixAmbiguousPromptFmt = `The value “%s” represents the output of a analog-meter-reading analysis performed on an image.
Uncertain digits within the reading are denoted by the “?” character.

Using the previously recorded meter value %f as a reference,
infer and replace the “?” characters to estimate the most probable complete reading.

Instructions:
- Return a string with the exact same length as the input value.
- Output only the predicted value, without any explanations or additional text.
`
