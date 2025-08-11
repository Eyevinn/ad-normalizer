package store

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/alicebob/miniredis/v2"
	"github.com/matryer/is"
)

var redisAdress string
var minir *miniredis.Miniredis

func TestMain(m *testing.M) {
	mr, err := miniredis.Run()
	if err != nil {
		log.Fatalf("an error was not expected when running miniredis: %s", err)
	}

	redisAdress = mr.Addr()
	minir = mr

	exitVal := m.Run()
	os.Exit(exitVal)
}

func TestValkeyStore(t *testing.T) {
	is := is.New(t)
	store, err := NewValkeyStore("redis://" + redisAdress)
	is.NoErr(err)
	testData := structure.TranscodeInfo{
		Url:         "http://example.com/video/index.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	err = store.Set("test-key", testData)
	is.NoErr(err)
	retrievedData, found, err := store.Get("test-key")
	is.NoErr(err)
	is.True(found)
	is.Equal(retrievedData.Url, testData.Url)
	is.Equal(retrievedData.AspectRatio, testData.AspectRatio)
	is.Equal(retrievedData.FrameRates, testData.FrameRates)
	is.Equal(retrievedData.Status, testData.Status)

	err = store.Delete("test-key")
	is.NoErr(err)
	_, found, err = store.Get("test-key")
	is.NoErr(err)
	is.True(!found)
}

func TestValkeyStoreTtl(t *testing.T) {
	is := is.New(t)
	store, err := NewValkeyStore("redis://" + redisAdress)
	is.NoErr(err)
	testData := structure.TranscodeInfo{
		Url:         "http://example.com/video/index.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	err = store.Set("test-key", testData, 1)
	is.NoErr(err)
	ttl, err := store.Ttl("test-key")
	is.NoErr(err)
	is.True(ttl > 0)
	minir.FastForward(2 * time.Second) // Key should expire after 1 second
	_, found, err := store.Get("test-key")
	is.NoErr(err)
	is.True(!found)                // Key should not be found after expiration
	_, err = store.Ttl("test-key") // Should return an error since key does not exist
	is.True(err != nil)
	err = store.Set("test-key", testData) // Set with no TTL
	is.NoErr(err)
	ttl, err = store.Ttl("test-key")
	is.NoErr(err)
	is.Equal(ttl, int64(-1))       // Key exists but has no TTL
	err = store.Delete("test-key") // Cleanup, should not error
	is.NoErr(err)
}

func TestQueuePackagingJob(t *testing.T) {
	is := is.New(t)
	store, err := NewValkeyStore("redis://" + redisAdress)
	is.NoErr(err)

	packagingJob := structure.PackagingQueueMessage{
		JobId: "test-job-id",
		Url:   "http://example-encore.osaas.io/encoreJobs/test-job-id",
	}

	err = store.EnqueuePackagingJob("test-queue", packagingJob)
	is.NoErr(err)
}
