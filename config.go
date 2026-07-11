package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/joho/godotenv"
)

// PromptPair is a system/user prompt pair loaded from YAML.
type PromptPair struct {
	System string `yaml:"system"`
	User   string `yaml:"user"`
}

// Config holds settings from environment variables and YAML (prompts).
type Config struct {
	MQTT struct {
		Host  string
		Topic string
	}
	Concierge struct {
		Addr  string
		Token string
	}
	// Gemini struct {
	// 	APIKey string
	// 	Model  string
	// }
	OpenAICompat struct {
		BaseURL string
		APIKey  string
		Model   string
	}
	ReadGasGauge PromptPair `yaml:"read_gas_gauge"`
	FixAmbiguous PromptPair `yaml:"fix_ambiguous"`
}

// LoadConfig reads prompt settings from YAML and connection secrets from the environment.
// It loads a local .env file if present (missing file is not an error).
func LoadConfig(filename string) (*Config, error) {
	_ = godotenv.Load()

	var config Config

	yamlFile, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer yamlFile.Close()

	decoder := yaml.NewDecoder(yamlFile)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode config file: %w", err)
	}

	config.MQTT.Host = os.Getenv("MQTT_HOST")
	config.MQTT.Topic = os.Getenv("MQTT_TOPIC")
	config.Concierge.Addr = os.Getenv("CONCIERGE_ADDR")
	config.Concierge.Token = os.Getenv("CONCIERGE_TOKEN")
	config.OpenAICompat.BaseURL = os.Getenv("OPENAI_BASE_URL")
	config.OpenAICompat.APIKey = os.Getenv("OPENAI_API_KEY")
	config.OpenAICompat.Model = os.Getenv("OPENAI_MODEL")

	if err := config.validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) validate() error {
	required := []struct {
		name, value string
	}{
		{"MQTT_HOST", c.MQTT.Host},
		{"MQTT_TOPIC", c.MQTT.Topic},
		{"CONCIERGE_ADDR", c.Concierge.Addr},
		{"CONCIERGE_TOKEN", c.Concierge.Token},
		{"OPENAI_BASE_URL", c.OpenAICompat.BaseURL},
		{"OPENAI_API_KEY", c.OpenAICompat.APIKey},
		{"OPENAI_MODEL", c.OpenAICompat.Model},
		{"read_gas_gauge.system", c.ReadGasGauge.System},
		{"read_gas_gauge.user", c.ReadGasGauge.User},
		{"fix_ambiguous.system", c.FixAmbiguous.System},
		{"fix_ambiguous.user", c.FixAmbiguous.User},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return fmt.Errorf("%s is required", r.name)
		}
	}
	return nil
}
