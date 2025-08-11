package osaas

import (
	"log/slog"

	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/logger"
	osaasclient "github.com/EyevinnOSC/client-go"
)

func SetupOsc(config *config.AdNormalizerConfig) (*osaasclient.Context, error) {
	logger.Debug("Setting up OSC client")
	oscConfig := &osaasclient.ContextConfig{
		PersonalAccessToken: config.OscToken,
		Environment:         config.Environment,
	}
	ctx, err := osaasclient.NewContext(oscConfig)
	if err != nil {
		logger.Error("Failed to create OSC client context", slog.String("error", err.Error()))
		return nil, err
	}
	logger.Info("OSC client context created successfully", slog.String("environment", config.Environment))
	return ctx, nil
}
