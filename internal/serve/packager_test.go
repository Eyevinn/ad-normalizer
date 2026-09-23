package serve

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/normalizerMetrics"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/matryer/is"
)

// setupApiWithEncoreError builds a minimal API whose EncoreHandler always fails
// GetEncoreJob with the supplied error.
func setupApiWithEncoreError(encoreErr error) *API {
	storeStub := &StoreStub{
		mockStore: make(map[string]structure.TranscodeInfo),
		kpis:      normalizerMetrics.NormalizerMetrics{},
	}
	adserverUrl, _ := url.Parse("http://localhost:9999")
	assetServerUrl, _ := url.Parse("https://asset-server.example.com")
	apiConf := config.AdNormalizerConfig{
		AdServerUrl:    *adserverUrl,
		AssetServerUrl: *assetServerUrl,
		KeyField:       "url",
		KeyRegex:       "[^a-zA-Z0-9]",
		InFlightTtl:    3600,
	}
	api := NewAPI(
		storeStub,
		apiConf,
		&EncoreHandlerGetJobErrorStub{err: encoreErr},
		&http.Client{},
		storeStub.kpiReport,
	)
	return api
}

func TestPackagingFailure(t *testing.T) {
	is := is.New(t)

	api, ts, storeStub, _ := setupApi()
	defer ts.Close()
	failureEvent := `{"message": {"jobId":"test-job-id","url":"http://encore-example.osaas.io/"}}`
	req, err := http.NewRequest("POST", "/failure", bytes.NewBufferString(failureEvent))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	api.HandlePackagingFailure(rr, req)
	is.Equal(rr.Code, http.StatusOK)
	is.Equal(storeStub.deletes, 1)

	storeStub.reset()
}

func TestPackagingSuccess(t *testing.T) {
	is := is.New(t)
	successEvent := `{
		"jobId": "test-job-id",
    	"url": "https://encore-instance",
    	"outputPath": "/output-folder/assetId/jobId/"
	}`
	api, ts, storeStub, _ := setupApi()
	defer ts.Close()
	req, err := http.NewRequest("POST", "/success", bytes.NewBufferString(successEvent))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	api.HandlePackagingSuccess(rr, req)
	is.Equal(rr.Code, http.StatusOK)
	is.Equal(storeStub.sets, 1)
	tci, ok, err := storeStub.Get("test-job-id")
	is.NoErr(err)
	is.True(ok)
	is.Equal(tci.Status, "COMPLETED")
	is.True(strings.HasSuffix(tci.Url, "index.m3u8"))
	storeStub.reset()
}

// TestPackagingSuccessGetEncoreJobError verifies that a GetEncoreJob failure
// (dependency/auth) returns 502 Bad Gateway, not 404.
func TestPackagingSuccessGetEncoreJobError(t *testing.T) {
	is := is.New(t)

	api := setupApiWithEncoreError(errors.New("401 Unauthorized: OSC SAT expired"))
	successEvent := `{"jobId":"test-job-id","outputPath":"/output/assetId/jobId/"}`
	req, err := http.NewRequest("POST", "/success", bytes.NewBufferString(successEvent))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	api.HandlePackagingSuccess(rr, req)
	is.Equal(rr.Code, http.StatusBadGateway)
}

// TestPackagingFailureGetEncoreJobError verifies that a GetEncoreJob failure
// (dependency/auth) returns 502 Bad Gateway, not 404.
func TestPackagingFailureGetEncoreJobError(t *testing.T) {
	is := is.New(t)

	api := setupApiWithEncoreError(errors.New("401 Unauthorized: OSC SAT expired"))
	failureEvent := `{"message":{"jobId":"test-job-id"}}`
	req, err := http.NewRequest("POST", "/failure", bytes.NewBufferString(failureEvent))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	api.HandlePackagingFailure(rr, req)
	is.Equal(rr.Code, http.StatusBadGateway)
}
