package health

import (
	"acgn-at-anyone-go/config"
	"acgn-at-anyone-go/internal/torrent"
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Health struct {
	config        *config.Config
	httpClient    *http.Client
	torrentClient *torrent.Client
}

type HealthStatus struct {
	DiskFree uint64 `json:"diskFree"`
	Port     int    `json:"port"`
}

func NewHealth(config *config.Config, torrentClient *torrent.Client) *Health {
	return &Health{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		torrentClient: torrentClient,
	}
}

func (h *Health) HeartbeatTicker() {
	if h.config.Health.Host == "" {
		log.Println("Health port not set, skip health check")
		return
	}
	ticker := time.NewTicker(time.Minute)
	go func() {
		for range ticker.C {
			h.Heartbeat()
		}
	}()
}

func (h *Health) Heartbeat() {
	status := HealthStatus{
		DiskFree: h.torrentClient.FreeCache(),
		Port:     h.config.Client.Port,
	}
	jsonData, err := json.Marshal(status)
	if err != nil {
		return
	}

	targetUrl := h.config.Health.Host + "torrent/heartbeat"

	req, err := http.NewRequest("POST", targetUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	return
}
