package main

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type Config struct {
	MQTT struct {
		Host  string `yaml:"host"`
		Topic string `yaml:"topic"`
	} `yaml:"mqtt"`
	Concierge struct {
		Addr  string `yaml:"addr"`
		Token string `yaml:"token"`
	} `yaml:"concierge"`
	Gemini struct {
		APIKey       string `yaml:"api_key"`
		Model        string `yaml:"model"`
		SystemPrompt string `yaml:"system_prompt"`
		Prompt       string `yaml:"prompt"`
	} `yaml:"gemini"`
}

func LoadConfig(filename string) (*Config, error) {
	var config Config

	yamlFile, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer yamlFile.Close()

	decoder := yaml.NewDecoder(yamlFile)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %v", err)
	}

	return &config, nil
}
