package main

import (
	"acgn-at-anyone-go/config"
	"acgn-at-anyone-go/internal/health"
	"acgn-at-anyone-go/internal/torrent"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {

	cfg, err := config.LoadConfig("config/config.yaml")
	if err != nil || cfg == nil {
		panic(err)
	}

	torrentClient := torrent.NewClient(&cfg.Torrent)
	healthClient := health.NewHealth(cfg, torrentClient)
	healthClient.HeartbeatTicker()
	torrentHandler := torrent.NewHandler(torrentClient)
	healthHandler := health.NewHandler(healthClient)

	r := gin.Default()

	r.POST("/seed", torrentHandler.AddMagnet)
	r.GET("/health", healthHandler.Ready)

	// 优雅启停
	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Client.Port),
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen failed: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown Server ...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server Shutdown Error:", err)
	}

	log.Println("Server exited")
}
