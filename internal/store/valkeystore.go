package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/valkey-io/valkey-go"
)

type Store interface {
	Get(key string) (structure.TranscodeInfo, bool, error)
	Set(key string, value structure.TranscodeInfo, ttl ...int) error
	Delete(key string) error
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

func (vs *ValkeyStore) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := vs.client.Do(ctx, vs.client.B().Del().Key(key).Build()).Error()
	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}
	return nil
}

func (vs *ValkeyStore) Get(key string) (structure.TranscodeInfo, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	value := structure.TranscodeInfo{}
	result, err := vs.client.Do(ctx, vs.client.B().Get().Key(key).Build()).AsBytes()
	if err != nil {
		return value, false, err
	}

	if len(result) == 0 {
		return value, false, errors.New("0 length value in valkey")
	}
	err = json.Unmarshal(result, &value)
	if err != nil {
		logger.Error("Failed to unmarshal value from Valkey", slog.String("key", key))
		return value, false, fmt.Errorf("failed to unmarshal value for key %s: %w", key, err)
	}
	return value, true, nil
}

func (vs *ValkeyStore) Set(key string, value structure.TranscodeInfo, ttl ...int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	valueBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
	}
	err = vs.client.Do(
		ctx,
		vs.client.B().Set().
			Key(key).
			Value(string(valueBytes)).
			Build()).
		Error()
	if err != nil {
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}
	if len(ttl) > 0 {
		ttlValue := ttl[0]

		err = vs.client.Do(
			ctx,
			vs.client.B().Expire().
				Key(key).
				Seconds(int64(ttlValue)).
				Build()).
			Error()
		if err != nil {
			return fmt.Errorf("failed to set TTL for key %s: %w", key, err)
		}
	} else {
		// make sure the key does not expire
		err = vs.client.Do(
			ctx,
			vs.client.B().Persist().
				Key(key).
				Build()).
			Error()
		if err != nil {
			return fmt.Errorf("failed to persist key %s: %w", key, err)
		}
		logger.Debug("Persisted key in Valkey", slog.String("key", key))
		logger.Debug("Set key in Valkey", slog.String("key", key))
	}
	return nil
}
