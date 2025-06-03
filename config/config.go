package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds configuration values loaded from nickcast.conf
type Config struct {
	ListenAddress string
	AuthURL       string
	APIToken      string
}

// AppConfig is the global config used throughout the application
var AppConfig Config

// LoadConfig reads nickcast.conf from the binary's directory
func LoadConfig() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("error finding executable path: %w", err)
	}

	configPath := filepath.Join(filepath.Dir(execPath), "nickcast.conf")

	file, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("error opening config file (%s): %w", configPath, err)
	}
	defer file.Close()

	cfg := Config{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "listen":
			cfg.ListenAddress = value
		case "auth_url":
			cfg.AuthURL = value
		case "api_token":
			cfg.APIToken = value
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	if cfg.ListenAddress == "" {
		cfg.ListenAddress = ":8000"
	}
	if cfg.AuthURL == "" {
		return fmt.Errorf("auth_url must be specified in nickcast.conf")
	}
	if cfg.APIToken == "" {
		return fmt.Errorf("api_token must be specified in nickcast.conf")
	}

	AppConfig = cfg
	return nil
}
