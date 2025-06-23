package config

import (
	"errors"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"golang.org/x/exp/slog"
)

type AdNormalizerConfig struct {
	EncoreUrl          string
	Bucket             string
	AdServerUrl        string
	ValkeyUrl          string
	ValkeyClusterUrl   string
	OscToken           string
	InFlightTtl        int
	KeyField           string
	KeyRegex           string
	encoreProfile      string
	JitPackage         bool
	PackagingQueueName string
	RootUrl            string
	BucketUrl          url.URL
	AssetServerUrl     string
}

func ReadConfig() (AdNormalizerConfig, error) {
	conf := AdNormalizerConfig{}
	encoreUrl, found := os.LookupEnv("ENCORE_URL")
	var err error
	if !found {
		logger.Error("No environment variable ENCORE_URL was found")
		err = errors.Join(err, errors.New("missing ENCORE_URL environment variable"))
	} else {
		conf.EncoreUrl = strings.TrimSuffix(encoreUrl, "/")
	}

	// TODO: log level should be configurable

	valkeyUrl, found := os.LookupEnv("VALKEY_URL")
	if !found {
		logger.Error("No environment variable VALKEY_URL was found")
		err = errors.Join(err, errors.New("missing VALKEY_URL environment variable"))
	} else {
		conf.ValkeyUrl = valkeyUrl
	}

	valkeyClusterUrl, found := os.LookupEnv("VALKEY_CLUSTER_URL")
	if !found {
		logger.Info("No environment variable VALKEY_CLUSTER_URL found")
	} else {
		conf.ValkeyClusterUrl = valkeyClusterUrl
	}

	adServerUrl, found := os.LookupEnv("AD_SERVER_URL")
	if !found {
		logger.Error("No environment variable AD_SERVER_URL was found")
		err = errors.Join(err, errors.New("missing AD_SERVER_URL environment variable"))
	} else {
		conf.AdServerUrl = adServerUrl
	}

	// TODO: configurable port

	bucketRaw, found := os.LookupEnv("OUTPUT_BUCKET_URL")
	if !found {
		logger.Error("No environment variable OUTPUT_BUCKET_URL was found")
		err = errors.Join(err, errors.New("missing OUTPUT_BUCKET_URL environment variable"))
	} else {
		bucket, err := url.Parse(strings.TrimSuffix(bucketRaw, "/"))
		if err != nil {
			logger.Error("Failed to parse OUTPUT_BUCKET_URL", slog.Error(err))
			err = errors.Join(err, errors.New("invalid OUTPUT_BUCKET_URL format"))
		} else {
			var bucketPath string
			if bucket.Path == "" {
				path.Join(bucket.Hostname(), bucket.Path)
			} else {
				bucketPath = bucket.Hostname()
			}
			conf.Bucket = bucketPath
			conf.BucketUrl = *bucket
		}
	}

	oscToken, found := os.LookupEnv("OSC_ACCESS_TOKEN")
	if !found {
		logger.Error("No environment variable OSC_ACCESS_TOKEN was found")
	} else {
		conf.OscToken = oscToken
	}

	keyField, found := os.LookupEnv("KEY_FIELD")
	if !found {
		logger.Error("No environment variable KEY_FIELD was found")
		conf.KeyField = "universalAdId"
	} else {
		conf.KeyField = keyField
	}

	keyRegex, found := os.LookupEnv("KEY_REGEX")
	if !found {
		logger.Error("No environment variable KEY_REGEX was found")
		conf.KeyRegex = "^[^a-zA-Z0-9]"
	} else {
		conf.KeyRegex = keyRegex
	}

	encoreProfile, found := os.LookupEnv("ENCORE_PROFILE")
	if !found {
		logger.Info("No environment variable ENCORE_PROFILE was found, using default")
		conf.encoreProfile = "program"
	} else {
		conf.encoreProfile = encoreProfile
	}

	assetServerUrl, found := os.LookupEnv("ASSET_SERVER_URL")
	if !found {
		logger.Error("No environment variable ASSET_SERVER_URL was found")
		err = errors.Join(err, errors.New("missing ASSET_SERVER_URL environment variable"))
	} else {
		conf.AssetServerUrl = strings.TrimSuffix(assetServerUrl, "/")
	}

	jitPackage, _ := os.LookupEnv("JIT_PACKAGE")

	conf.JitPackage = jitPackage == "true"

	return conf, err
}
