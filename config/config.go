package config

import (
	"log"
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Client  ClientConfig  `yaml:"client"`
	Health  HealthConfig  `yaml:"health"`
	Torrent TorrentConfig `yaml:"torrent"`
}

type ClientConfig struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type HealthConfig struct {
	Host string `yaml:"host"`
}

type TorrentConfig struct {
	DataDir       string        `yaml:"path"`
	MaxDisk       uint64        `yaml:"max_disk_usage"`
	CleanInterval time.Duration `yaml:"clean_interval"`
	Trackers      []string      `yaml:"trackers"`
	ICEServers    []string      `yaml:"ice_servers"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		log.Printf("failed to open config file: %v", err)
		return nil, nil
	}
	defer file.Close()

	var config Config
	err = yaml.NewDecoder(file).Decode(&config)
	if err != nil {
		log.Printf("failed to open config file: %v", err)
		return nil, nil
	}

	return &config, nil
}
