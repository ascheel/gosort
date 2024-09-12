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
	Port    int    `yaml:"port"`
}

type ClientConfig struct {
	Host string `yaml:"host"`
}

func LoadConfig() (*Config, error) {
	var c Config
	configFile := filepath.Join("config.yml")
	f, err := os.Open(configFile)
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
