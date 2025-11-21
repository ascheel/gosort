package sortengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	Client ClientConfig `yaml:"client"`
}

type ServerConfig struct {
	DBFile  string `yaml:"database_file"`
	SaveDir string `yaml:"savedir"`
	IP      string `yaml:"ip"`
	Port    int    `yaml:"port"`
}

type ClientConfig struct {
	Host string `yaml:"host"`
}

// ConfigFlags holds command-line flag values that can override config file settings
type ConfigFlags struct {
	ConfigFile  string
	DBFile      string
	SaveDir     string
	IP          string
	Port        int
	Host        string
	InitConfig  bool
}

// GetDefaultConfigPath returns the default config file path (~/.gosort.yml)
func GetDefaultConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine home directory: %v", err)
	}
	return filepath.Join(homeDir, ".gosort.yml"), nil
}

// CreateDefaultConfig creates a default config file at the specified path
func CreateDefaultConfig(configPath string) error {
	_, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to determine home directory: %v", err)
	}

	defaultConfig := Config{
		Server: ServerConfig{
			DBFile:  "%SAVEDIR%/gosort.db",
			SaveDir: "%HOME%/pictures",
			IP:      "localhost",
			Port:    8080,
		},
		Client: ClientConfig{
			Host: "localhost:8080",
		},
	}

	// Ensure the directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("unable to create config directory: %v", err)
	}

	// Create the config file
	f, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("unable to create config file: %v", err)
	}
	defer f.Close()

	encoder := yaml.NewEncoder(f)
	defer encoder.Close()

	if err := encoder.Encode(defaultConfig); err != nil {
		return fmt.Errorf("unable to encode config file: %v", err)
	}

	fmt.Printf("Created default config file at: %s\n", configPath)
	return nil
}

// LoadConfig loads the config file from the specified path, or default location if empty
func LoadConfig(configPath string) (*Config, error) {
	var c Config

	// Use default path if not specified
	if configPath == "" {
		var err error
		configPath, err = GetDefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s (use -init to create it)", configPath)
	}

	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("unable to open config file: %v", err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&c)
	if err != nil {
		return nil, fmt.Errorf("unable to decode config file: %v", err)
	}

	// Find %HOME% and replace with the actual home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("unable to determine home directory: %v", err)
	}
	c.Server.SaveDir = strings.Replace(c.Server.SaveDir, "%HOME%", homeDir, 1)
	c.Server.DBFile = strings.Replace(c.Server.DBFile, "%SAVEDIR%", c.Server.SaveDir, 1)

	return &c, nil
}

// ApplyFlags applies command-line flags to the config, overriding file values
func (c *Config) ApplyFlags(flags *ConfigFlags) {
	if flags.DBFile != "" {
		c.Server.DBFile = flags.DBFile
		// Replace %SAVEDIR% if present
		c.Server.DBFile = strings.Replace(c.Server.DBFile, "%SAVEDIR%", c.Server.SaveDir, 1)
	}
	if flags.SaveDir != "" {
		c.Server.SaveDir = flags.SaveDir
		// Replace %HOME% if present
		homeDir, err := os.UserHomeDir()
		if err == nil {
			c.Server.SaveDir = strings.Replace(c.Server.SaveDir, "%HOME%", homeDir, 1)
		}
		// Update DBFile if it uses %SAVEDIR%
		if strings.Contains(c.Server.DBFile, "%SAVEDIR%") {
			c.Server.DBFile = strings.Replace(c.Server.DBFile, "%SAVEDIR%", c.Server.SaveDir, 1)
		}
	}
	if flags.IP != "" {
		c.Server.IP = flags.IP
	}
	if flags.Port > 0 {
		c.Server.Port = flags.Port
	}
	if flags.Host != "" {
		c.Client.Host = flags.Host
	}
}
