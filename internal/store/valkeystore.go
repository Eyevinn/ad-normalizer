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
	Set(key string, value structure.TranscodeInfo, ttl ...int64) error
	Delete(key string) error
	EnqueuePackagingJob(queueName string, packagingJob structure.PackagingQueueMessage) error
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
		if errors.Is(err, valkey.Nil) {
			return value, false, nil // Key does not exist
		}
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

func (vs *ValkeyStore) Set(key string, value structure.TranscodeInfo, ttl ...int64) error {
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
		logger.Debug("Set key in Valkey",
			slog.String("key", key),
			slog.String("url", value.Url),
			slog.String("status", value.Status),
		)
	}
	return nil
}

func (vs *ValkeyStore) Ttl(key string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	result, err := vs.client.Do(ctx, vs.client.B().Ttl().Key(key).Build()).AsInt64()
	if err != nil {
		logger.Warn("Could not get TTL from valkey", slog.String("key", key), slog.String("err", err.Error()))
		return 0, err
	}
	switch result {
	case -2: // key does not exist
		return 0, errors.New("key does not exist")
	case -1: // key exists but has no TTL
		return -1, nil
	default: // key exists and has a TTL
		return result, nil
	}
}

func (vs *ValkeyStore) EnqueuePackagingJob(queueName string, packagingJob structure.PackagingQueueMessage) error {
	serializedJob, err := json.Marshal(packagingJob)
	if err != nil {
		return fmt.Errorf("failed to serialize packaging job %s: %w", packagingJob.JobId, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = vs.client.Do(
		ctx,
		vs.client.B().
			Zadd().
			Key(queueName).
			ScoreMember().
			ScoreMember(float64(time.Now().UnixMilli()), string(serializedJob)).
			Build()).
		Error()
	if err != nil {
		return fmt.Errorf("failed to enqueue packaging job %s: %w", packagingJob, err)
	}
	return nil
}
