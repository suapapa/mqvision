package concierge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

type Client struct {
	addr  string
	token string
}

func NewClient(addr string, token string) *Client {
	return &Client{
		addr:  addr,
		token: token,
	}
}

func (c *Client) PostImage(image io.Reader, mimeType string) (string, error) {
	// curl -X POST http://localhost:8080/api/v1/luggage \
	// -F "file=@image.png" \
	// -F "mime=image/png" \
	// -F "ttl=10"

	url := fmt.Sprintf("%s/api/v1/luggage", c.addr)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add file field
	fileWriter, err := writer.CreateFormFile("file", "image")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(fileWriter, image)
	if err != nil {
		return "", err
	}

	// Add mime field
	writer.WriteField("mime", mimeType)

	// Add ttl field
	writer.WriteField("ttl", fmt.Sprintf("%d", 60*24*2)) // 2 days

	err = writer.Close()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("concierge server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Key string `json:"key"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	if result.Key == "" {
		return "", fmt.Errorf("concierge response did not contain a key")
	}

	return c.addr + "/api/v1/luggage/" + result.Key, nil
}
