package config

import (
	"testing"

	"github.com/matryer/is"
)

func TestReadConfig(t *testing.T) {
	is := is.New(t)
	configVars := []struct {
		name  string
		value string
	}{
		{"ENCORE_URL", "http://demo-encore.osaas.io"},
		{"REDIS_URL", "redis://demo-valkey.osaas.io"},
		{"REDIS_CLUSTER", "true"},
		{"AD_SERVER_URL", "http://test-ad-server.osaas.io"},
		{"OUTPUT_BUCKET_URL", "s3://test-bucket.osaas.io"},
		{"KEY_FIELD", "url"},
		{"KEY_REGEX", "^[^a-zA-Z0-9]"},
		{"ENCORE_PROFILE", "ad-profile"},
		{"ASSET_SERVER_URL", "http://test-asset-server.osaas.io"},
		{"JIT_PACKAGE", "true"},
		{"ROOT_URL", "http://ad-normalizer.osaas.io"},
		{"PACKAGING_QUEUE", "normalizer-package"},
		{"IN_FLIGHT_TTL", "10"},
		{"PPROF_PORT", "6060"},
	}
	for _, v := range configVars {
		t.Setenv(v.name, v.value)
	}
	config, err := ReadConfig()
	is.NoErr(err)
	is.Equal(config.EncoreUrl.String(), "http://demo-encore.osaas.io")
	is.Equal(config.ValkeyUrl, "redis://demo-valkey.osaas.io")
	is.Equal(config.AdServerUrl.String(), "http://test-ad-server.osaas.io")
	is.Equal(config.BucketUrl.String(), "s3://test-bucket.osaas.io")
	is.Equal(config.InFlightTtl, 10)
	is.Equal(config.KeyField, "url")
	is.Equal(config.KeyRegex, "^[^a-zA-Z0-9]")
	is.Equal(config.EncoreProfile, "ad-profile")
	is.Equal(config.ValkeyCluster, true)
	is.Equal(config.PProfPort, "6060")
}

func TestPProfPortNotSet(t *testing.T) {
	is := is.New(t)
	configVars := []struct {
		name  string
		value string
	}{
		{"ENCORE_URL", "http://demo-encore.osaas.io"},
		{"REDIS_URL", "redis://demo-valkey.osaas.io"},
		{"AD_SERVER_URL", "http://test-ad-server.osaas.io"},
		{"OUTPUT_BUCKET_URL", "s3://test-bucket.osaas.io"},
		{"ASSET_SERVER_URL", "http://test-asset-server.osaas.io"},
		{"ROOT_URL", "http://ad-normalizer.osaas.io"},
	}
	for _, v := range configVars {
		t.Setenv(v.name, v.value)
	}
	config, err := ReadConfig()
	is.NoErr(err)
	is.Equal(config.PProfPort, "")
}

func TestPProfPortInvalid(t *testing.T) {
	is := is.New(t)
	configVars := []struct {
		name  string
		value string
	}{
		{"ENCORE_URL", "http://demo-encore.osaas.io"},
		{"REDIS_URL", "redis://demo-valkey.osaas.io"},
		{"AD_SERVER_URL", "http://test-ad-server.osaas.io"},
		{"OUTPUT_BUCKET_URL", "s3://test-bucket.osaas.io"},
		{"ASSET_SERVER_URL", "http://test-asset-server.osaas.io"},
		{"ROOT_URL", "http://ad-normalizer.osaas.io"},
		{"PPROF_PORT", "not-a-port"},
	}
	for _, v := range configVars {
		t.Setenv(v.name, v.value)
	}
	config, err := ReadConfig()
	is.NoErr(err)
	is.Equal(config.PProfPort, "6060")
}

func TestPProfPortOutOfRange(t *testing.T) {
	is := is.New(t)
	configVars := []struct {
		name  string
		value string
	}{
		{"ENCORE_URL", "http://demo-encore.osaas.io"},
		{"REDIS_URL", "redis://demo-valkey.osaas.io"},
		{"AD_SERVER_URL", "http://test-ad-server.osaas.io"},
		{"OUTPUT_BUCKET_URL", "s3://test-bucket.osaas.io"},
		{"ASSET_SERVER_URL", "http://test-asset-server.osaas.io"},
		{"ROOT_URL", "http://ad-normalizer.osaas.io"},
		{"PPROF_PORT", "99999"},
	}
	for _, v := range configVars {
		t.Setenv(v.name, v.value)
	}
	config, err := ReadConfig()
	is.NoErr(err)
	is.Equal(config.PProfPort, "6060")
}
