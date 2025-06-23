package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Eyevinn/ad-normalizer/internal/logger"
)

func main() {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	mainmux := http.NewServeMux()

	mainmux.HandleFunc("/ping", healthCheck)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mainmux,
	}

	go func() {
		logger.Info("Starting server...")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logger.Error("Failed to start server", slog.String("error", err.Error()))
			panic(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit
	logger.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error", slog.String("error", err.Error()))
		stop()
		panic(err)
	} else {
		logger.Info("Server gracefully stopped")
	}
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("pong"))
}
