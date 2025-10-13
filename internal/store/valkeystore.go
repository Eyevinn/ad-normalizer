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

const BLACKLIST_KEY = "blacklist"
const TIME_INDEX_KEY = "job_time_index"

type Store interface {
	Get(key string) (structure.TranscodeInfo, bool, error)
	Set(key string, value structure.TranscodeInfo, ttl ...int64) error
	Delete(key string) error
	EnqueuePackagingJob(queueName string, packagingJob structure.PackagingQueueMessage) error
	BlackList(value string) error
	InBlackList(value string) (bool, error)
	RemoveFromBlackList(value string) error
	List(page int, size int) ([]structure.TranscodeInfo, int64, error)
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
	err = deleteFromTimeIndex(vs, key)
	if err != nil {
		return fmt.Errorf("failed to delete key %s from time index: %w", key, err)
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
	err = vs.updateTimeIndex(key)
	if err != nil {
		return fmt.Errorf("failed to update time index for key %s: %w", key, err)
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

func (vs *ValkeyStore) BlackList(value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := vs.client.Do(
		ctx,
		vs.client.B().
			Zadd().
			Key(BLACKLIST_KEY).
			ScoreMember().
			ScoreMember(float64(time.Now().UnixMilli()), value).
			Build()).
		Error()
	if err != nil {
		return fmt.Errorf("failed to add key %s to blacklist: %w", value, err)
	}
	logger.Info("Added URL to blacklist", slog.String("key", value))
	return nil
}

func (vs *ValkeyStore) InBlackList(value string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := vs.client.Do(ctx, vs.client.B().Zscore().Key(BLACKLIST_KEY).Member(value).Build()).AsFloat64()
	if err != nil {
		if errors.Is(err, valkey.Nil) {
			return false, nil // Key is not in blacklist
		}
		return false, fmt.Errorf("failed to check if key %s is in blacklist: %w", value, err)
	}
	return true, nil // If score is >= 0, the key is in the blacklist
}

func (vs *ValkeyStore) RemoveFromBlackList(value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := vs.client.Do(
		ctx,
		vs.client.B().
			Zrem().
			Key(BLACKLIST_KEY).
			Member(value).
			Build()).
		Error()

	if err != nil {
		return fmt.Errorf("failed to remove key %s from blacklist: %w", value, err)
	}
	logger.Info("Removed URL from blacklist", slog.String("key", value))
	return nil
}

func (vs *ValkeyStore) updateTimeIndex(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := vs.client.Do(
		ctx,
		vs.client.B().
			Zadd().
			Key(TIME_INDEX_KEY).
			ScoreMember().
			ScoreMember(float64(time.Now().UnixMilli()), key).
			Build()).
		Error()
	if err != nil {
		return fmt.Errorf("failed to update time index for key %s: %w", key, err)
	}
	return err
}

func deleteFromTimeIndex(vs *ValkeyStore, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := vs.client.Do(
		ctx,
		vs.client.B().
			Zrem().
			Key(TIME_INDEX_KEY).
			Member(key).
			Build()).
		Error()
	if err != nil {
		return fmt.Errorf("failed to delete key %s from time index: %w", key, err)
	}
	return err
}

func (vs *ValkeyStore) List(page int, size int) ([]structure.TranscodeInfo, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := int64(page * size) // 1-index to 0-index
	end := int64(start + int64(size) - 1)
	keys, err := vs.client.Do(
		ctx,
		vs.client.B().
			Zrevrange().
			Key(TIME_INDEX_KEY).
			Start(start).
			Stop(end).
			Build()).AsStrSlice()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get keys from time index: %w", err)
	}
	cardinality, err := vs.client.Do(
		ctx,
		vs.client.B().
			Zcard().
			Key(TIME_INDEX_KEY).
			Build()).AsInt64()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get cardinality of time index: %w", err)
	}
	results := make([]structure.TranscodeInfo, 0, len(keys))
	mGetRes, err := valkey.MGet(vs.client, ctx, keys)
	for _, key := range keys {
		data, ok := mGetRes[key]
		if !ok {
			continue // Key does not exist
		}
		// TODO: Error handling
		bytesData, _ := data.AsBytes()
		var value structure.TranscodeInfo
		err = json.Unmarshal(bytesData, &value)
		if err != nil {
			logger.Error("Failed to unmarshal value from Valkey", slog.String("key", key))
			continue
		}
		results = append(results, value)
	}
	return results, cardinality, err
}
