package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/valkey-io/valkey-go"
)

type Store interface {
	Get(key string) (string, bool, error)
	Set(key string, value string, ttl ...int) error
}

type ValkeyStore struct {
	client valkey.Client
}

func NewValkeyStore(valkeyUrl string) (*ValkeyStore, error) {
	logger.Debug("Connecting to Valkey", slog.String("valkeyUrl", valkeyUrl))
	options := valkey.MustParseURL(valkeyUrl)
	options.SendToReplicas = func(cmd valkey.Completed) bool {
		return false // No read from replicas
	}
	options.DisableCache = true
	client, err := valkey.NewClient(options)
	if err != nil {
		logger.Error("Failed to create Valkey client", slog.String("error", err.Error()))
		return nil, err
	}
	return &ValkeyStore{
		client: client,
	}, nil
}

func (vs *ValkeyStore) Get(key string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var value string
	result, err := vs.client.Do(ctx, vs.client.B().Get().Key(key).Build()).AsBytes()
	if err != nil {
		return value, false, err
	}

	if len(result) == 0 {
		return value, false, errors.New("0 length value in valkey")
	}

	return value, true, nil
}

func (vs *ValkeyStore) Set(key string, value string, ttl ...int) error {
	ttlValue := 3600 // Default TTL of 1 hour
	if len(ttl) > 0 {
		ttlValue = ttl[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := vs.client.Do(
		ctx,
		vs.client.B().Set().
			Key(key).
			Value(value).
			Ex(time.Second*time.Duration(ttlValue)).
			Build()).
		Error()
	if err != nil {
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}
	return nil
}
