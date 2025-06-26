package main

import (
	"compress/gzip"
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/encore"
	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/serve"
	"github.com/Eyevinn/ad-normalizer/internal/store"
	"github.com/klauspost/compress/gzhttp"
)

func main() {
	// TODO: Telemetry
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	config, err := config.ReadConfig()
	if err != nil {
		logger.Error("Failed to read configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}
	api, err := setupApi(&config)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/vmap", api.HandleVmap)
	apiMux.HandleFunc("/vast", api.HandleVast)

	apiMuxChain := setupMiddleWare(apiMux, "api")
	mainmux := http.NewServeMux()

	mainmux.HandleFunc("/encoreCallback", api.HandleEncoreCallback)
	mainmux.HandleFunc("/ping", healthCheck)
	mainmux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiMuxChain))

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

func corsMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Add("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Access-Control-Expose-Headers", "Set-Cookie")
		w.Header().Add("Access-Control-Allow-Origin", origin)
		next.ServeHTTP(w, r)
	}
}

func recovery(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("There was a panic in a request",
					slog.Any("Message", err),
					slog.String("stack", string(debug.Stack())),
				)
				w.WriteHeader(500)
			}
		}()

		next.ServeHTTP(w, r)
	}
}

func setupMiddleWare(mainHandler http.Handler, name string) http.Handler {
	compressorMiddleware, err := gzhttp.NewWrapper(gzhttp.MinSize(2000), gzhttp.CompressionLevel(gzip.BestSpeed))
	if err != nil {
		panic(err)
	}
	return recovery(corsMiddleware(compressorMiddleware(mainHandler)))
}

func setupApi(config *config.AdNormalizerConfig) (*serve.API, error) {
	valkeyConnectionUrl := ""
	if config.ValkeyClusterUrl != "" {
		valkeyConnectionUrl = config.ValkeyClusterUrl
	} else {
		valkeyConnectionUrl = config.ValkeyUrl
	}
	valkeyStore, err := store.NewValkeyStore(valkeyConnectionUrl)

	client := &http.Client{}
	encoreHandler := &encore.HttpEncoreHandler{
		Client: client,
	}

	if err != nil {
		logger.Error("Failed to create Valkey store", slog.String("error", err.Error()))
		return nil, err
	}
	logger.Debug("Valkey store created successfully")
	api := serve.NewAPI(valkeyStore, *config, encoreHandler, client)
	return api, nil
}
