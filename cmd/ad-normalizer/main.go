package main

import (
	"compress/gzip"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/Eyevinn/ad-normalizer/cmd/ad-normalizer/telemetry"
	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/encore"
	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/osaas"
	"github.com/Eyevinn/ad-normalizer/internal/serve"
	"github.com/Eyevinn/ad-normalizer/internal/store"
	osaasclient "github.com/EyevinnOSC/client-go"
	"github.com/joho/godotenv"
	"github.com/klauspost/compress/gzhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"

	_ "net/http/pprof" // Import pprof for profiling
)

func main() {
	err := godotenv.Load()
	if err != nil {
		logger.Debug("Error loading .env file", slog.String("error", err.Error()))
	}
	config, err := config.ReadConfig()
	if err != nil {
		logger.Error("Failed to read configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	otelShutdown, err := telemetry.SetupOtelSdk(ctx, config)

	if err != nil {
		return
	}

	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	if err != nil {
		logger.Error("Failed to read configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}
	api, err := setupApi(&config)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/vmap", api.HandleVmap)
	apiMux.HandleFunc("/vast", api.HandleVast)
	apiMux.HandleFunc("/blacklist", api.HandleBlackList)

	packagerMux := http.NewServeMux()
	packagerMux.HandleFunc("/success", api.HandlePackagingSuccess)
	packagerMux.HandleFunc("/failure", api.HandlePackagingFailure)

	apiMuxChain := setupMiddleWare(apiMux, "api")
	packagerMuxChain := setupMiddleWare(packagerMux, "packager")
	mainmux := http.NewServeMux()

	mainmux.HandleFunc("/encoreCallback", api.HandleEncoreCallback)
	mainmux.HandleFunc("/ping", healthCheck)
	mainmux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiMuxChain))
	mainmux.Handle("/packagerCallback/", http.StripPrefix("/packagerCallback", packagerMuxChain))

	//Do not expose pprod debug endpoints in production
	if config.Environment != "PRODUCTION" {
		mainmux.Handle("/debug/", http.DefaultServeMux)
	}

	server := &http.Server{
		Addr:    ":" + strconv.Itoa(config.Port),
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
					slog.Any("message", err),
					slog.String("stack", string(debug.Stack())),
				)
				w.WriteHeader(500)
			}
		}()

		next.ServeHTTP(w, r)
	}
}

func setupMiddleWare(mainHandler http.Handler, name string) http.Handler {
	otelMiddleware := otelhttp.NewMiddleware(name, otelhttp.WithPropagators(otel.GetTextMapPropagator()))
	compressorMiddleware, err := gzhttp.NewWrapper(gzhttp.MinSize(2000), gzhttp.CompressionLevel(gzip.BestSpeed))
	if err != nil {
		panic(err)
	}
	return recovery(otelMiddleware(corsMiddleware(compressorMiddleware(mainHandler))))
}

func setupApi(config *config.AdNormalizerConfig) (*serve.API, error) {

	valkeyStore, err := store.NewValkeyStore(config.ValkeyUrl)
	var oscCtx *osaasclient.Context
	if config.OscToken != "" {
		oscCtx, err = osaas.SetupOsc(config)
		if err != nil {
			logger.Error("Failed to setup OSC client", slog.String("error", err.Error()))
			return nil, err
		}
	}
	client := &http.Client{}
	encoreHandler := encore.NewHttpEncoreHandler(
		http.DefaultClient,
		config.EncoreUrl,
		config.EncoreProfile,
		oscCtx,
		config.BucketUrl,
		config.RootUrl,
	)

	if err != nil {
		logger.Error("Failed to create Valkey store", slog.String("error", err.Error()))
		return nil, err
	}
	logger.Debug("Valkey store created successfully")
	api := serve.NewAPI(valkeyStore, *config, encoreHandler, client)
	return api, nil
}
