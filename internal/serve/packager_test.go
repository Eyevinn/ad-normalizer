package serve

import (
	"bytes"
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

func TestPackagingFailure(t *testing.T) {
	is := is.New(t)

	ss := &StoreStub{
		mockStore: make(map[string]structure.TranscodeInfo),
		kpis:      normalizerMetrics.NormalizerMetrics{},
	}

	ts = setupTestServer()
	defer ts.Close()
	eh := &EncoreHandlerStub{}
	adserverUrl, _ := url.Parse(ts.URL)
	assetServerUrl, _ := url.Parse("https://asset-server.example.com")
	ac := config.AdNormalizerConfig{
		AdServerUrl:    *adserverUrl,
		AssetServerUrl: *assetServerUrl,
		KeyField:       "url",
		KeyRegex:       "[^a-zA-Z0-9]",
		KpiPostUrl:     "http://kpi-post.example.com/metrics",
	}
	// Initialize the API with the mock store
	runApiInstance := NewAPI(
		ss,
		ac,
		eh,
		&http.Client{}, // Use nil for the client in tests, or you can create a mock client
		ss.kpiReport,
	)
	failureEvent := `{"message": {"jobId":"test-job-id","url":"http://encore-example.osaas.io/"}}`
	req, err := http.NewRequest("POST", "/failure", bytes.NewBufferString(failureEvent))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	runApiInstance.HandlePackagingFailure(rr, req)
	is.Equal(rr.Code, http.StatusOK)
	is.Equal(ss.deletes, 1)

	ss.reset()
}

func TestPackagingSuccess(t *testing.T) {
	is := is.New(t)
	successEvent := `{
		"jobId": "test-job-id",
    	"url": "https://encore-instance",
    	"outputPath": "/output-folder/assetId/jobId/"
	}`
	ss := &StoreStub{
		mockStore: make(map[string]structure.TranscodeInfo),
		kpis:      normalizerMetrics.NormalizerMetrics{},
	}

	ts = setupTestServer()
	defer ts.Close()
	eh := &EncoreHandlerStub{}
	adserverUrl, _ := url.Parse(ts.URL)
	assetServerUrl, _ := url.Parse("https://asset-server.example.com")
	ac := config.AdNormalizerConfig{
		AdServerUrl:    *adserverUrl,
		AssetServerUrl: *assetServerUrl,
		KeyField:       "url",
		KeyRegex:       "[^a-zA-Z0-9]",
		KpiPostUrl:     "http://kpi-post.example.com/metrics",
	}
	// Initialize the API with the mock store
	runApiInstance := NewAPI(
		ss,
		ac,
		eh,
		&http.Client{}, // Use nil for the client in tests, or you can create a mock client
		ss.kpiReport,
	)
	req, err := http.NewRequest("POST", "/success", bytes.NewBufferString(successEvent))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	runApiInstance.HandlePackagingSuccess(rr, req)
	is.Equal(rr.Code, http.StatusOK)
	is.Equal(ss.sets, 1)
	tci, ok, err := ss.Get("test-job-id")
	is.NoErr(err)
	is.True(ok)
	is.Equal(tci.Status, "COMPLETED")
	is.True(strings.HasSuffix(tci.Url, "index.m3u8"))
	ss.reset()
}
