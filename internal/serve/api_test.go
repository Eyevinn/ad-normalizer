package serve

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Eyevinn/VMAP/vmap"
	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/normalizerMetrics"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/Eyevinn/ad-normalizer/internal/util"
	"github.com/google/uuid"
	"github.com/matryer/is"
)

type StoreStub struct {
	mu        sync.Mutex
	mockStore map[string]structure.TranscodeInfo
	sets      int
	gets      int
	deletes   int
	blacklist []string
	kpis      normalizerMetrics.NormalizerMetrics
	lastTtls  map[string]int64 // keyed by cache key; -1 means no TTL was passed
}

func (s *StoreStub) kpiReport(args normalizerMetrics.AdsHandledEventArguments) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kpis.BrokenAds += args.BrokenAds
	s.kpis.IngestedAds += args.IngestedAds
	s.kpis.ServedAds += args.ServedAds
}

// Delete implements store.Store.
func (s *StoreStub) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mockStore, key)
	s.deletes++
	return nil
}

func (s *StoreStub) Get(key string) (structure.TranscodeInfo, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets++
	if value, exists := s.mockStore[key]; exists {
		return value, true, nil
	}
	return structure.TranscodeInfo{}, false, nil
}

func (s *StoreStub) Set(key string, value structure.TranscodeInfo, ttl ...int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sets++
	s.mockStore[key] = value
	if s.lastTtls == nil {
		s.lastTtls = make(map[string]int64)
	}
	if len(ttl) > 0 {
		s.lastTtls[key] = ttl[0]
	} else {
		s.lastTtls[key] = -1
	}
	return nil
}

func (s *StoreStub) List(page int, size int) ([]structure.TranscodeInfo, int64, error) {
	result := make([]structure.TranscodeInfo, 0, size)
	for i := range size {
		strVal := strconv.Itoa((page * size) + (size - 1 - i))
		tci := structure.TranscodeInfo{
			Url:        "http://example.com/video" + strVal + "/index.m3u8",
			Status:     "COMPLETED",
			Source:     "s3://fake-bucket/video" + strVal + ".mp4",
			LastUpdate: time.Now().Unix(),
		}
		result = append(result, tci)
	}
	return result, int64(size), nil
}

func (s *StoreStub) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mockStore = make(map[string]structure.TranscodeInfo)
	s.sets = 0
	s.gets = 0
	s.deletes = 0
	s.kpis = normalizerMetrics.NormalizerMetrics{}
	s.blacklist = []string{} // Reset the blacklist
	s.lastTtls = make(map[string]int64)
}

func (s *StoreStub) BlackList(key string) error {
	s.blacklist = append(s.blacklist, key)
	return nil
}

func (s *StoreStub) InBlackList(key string) (bool, error) {
	if slices.Contains(s.blacklist, key) {
		return true, nil
	}
	return false, nil
}

func (s *StoreStub) RemoveFromBlackList(key string) error {
	for i, blacklistedKey := range s.blacklist {
		if blacklistedKey == key {
			s.blacklist = append(s.blacklist[:i], s.blacklist[i+1:]...)
			return nil
		}
	}
	return nil // Key not found in blacklist, nothing to remove
}

func (s *StoreStub) GetBlackList(page int, size int) ([]string, int64, error) {
	return s.blacklist, int64(len(s.blacklist)), nil
}

func (s *StoreStub) EnqueuePackagingJob(queueName string, message structure.PackagingQueueMessage) error {
	// This is a stub, in a real implementation this would enqueue the job to a queue
	return nil
}

type EncoreHandlerStub struct {
	mu    sync.Mutex
	calls int
}

// GetEncoreJob implements encore.EncoreHandler.
func (e *EncoreHandlerStub) GetEncoreJob(jobId string) (structure.EncoreJob, error) {
	return structure.EncoreJob{
		Id:         uuid.NewString(),
		ExternalId: jobId,
		Profile:    "test-profile",
		BaseName:   jobId,
		Status:     "COMPLETED",
		Outputs: []structure.EncoreOutput{
			{
				MediaType: "Video",
				VideoStreams: []structure.EncoreVideoStream{
					{
						Codec:     "AVC",
						Width:     1920,
						Height:    1080,
						FrameRate: "25",
					},
				},
			},
			{
				MediaType: "Audio",
				AudioStreams: []structure.EncoreAudioStream{
					{
						Codec:    "AAC",
						Channels: 2,
					},
				},
			},
		},
		Inputs: []structure.EncoreInput{
			{
				Uri: "http://example.com/source/video.mp4",
			},
		},
	}, nil
}

func (e *EncoreHandlerStub) reset() {
	logger.Info("Resetting EncoreHandlerStub")
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = 0
}

func (e *EncoreHandlerStub) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	logger.Info("EncoreHandlerStub.createJob called")
	newJob := structure.EncoreJob{}
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	return newJob, nil
}

// EncoreHandlerFailStub always returns an error from CreateJob.
type EncoreHandlerFailStub struct{}

func (e *EncoreHandlerFailStub) GetEncoreJob(jobId string) (structure.EncoreJob, error) {
	return structure.EncoreJob{}, nil
}

func (e *EncoreHandlerFailStub) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	return structure.EncoreJob{}, errors.New("encore submission rejected: 503 Service Unavailable")
}

// TestDispatchJobsDoesNotWriteQueuedMarkerOnSubmitError verifies that when
// Encore submission fails the QUEUED marker is NOT written to the cache.
// Before the fix CreateJob swallowed the error, so the marker was always written.
func TestDispatchJobsDoesNotWriteQueuedMarkerOnSubmitError(t *testing.T) {
	is := is.New(t)

	storeStub := &StoreStub{
		mockStore: make(map[string]structure.TranscodeInfo),
		lastTtls:  make(map[string]int64),
		kpis:      normalizerMetrics.NormalizerMetrics{},
	}
	adserverUrl, _ := url.Parse("http://localhost:9999") // unused in this test
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
		&EncoreHandlerFailStub{},
		&http.Client{},
		storeStub.kpiReport,
	)

	creative := structure.ManifestAsset{
		CreativeId:        "fail-creative-id",
		MasterPlaylistUrl: "http://example.com/ad.mp4",
	}
	api.dispatchJobs(map[string]structure.ManifestAsset{creative.CreativeId: creative})

	// Give the goroutines a moment to complete.
	time.Sleep(100 * time.Millisecond)

	// No marker should have been written because submission failed.
	is.Equal(storeStub.sets, 0)
}

// TestDispatchJobsWritesQueuedMarkerWithTtl verifies that on a successful
// submission the QUEUED marker is stored with the configured IN_FLIGHT_TTL so
// that stranded markers self-heal instead of wedging the creative permanently.
func TestDispatchJobsWritesQueuedMarkerWithTtl(t *testing.T) {
	is := is.New(t)

	storeStub := &StoreStub{
		mockStore: make(map[string]structure.TranscodeInfo),
		lastTtls:  make(map[string]int64),
		kpis:      normalizerMetrics.NormalizerMetrics{},
	}
	adserverUrl, _ := url.Parse("http://localhost:9999") // unused in this test
	assetServerUrl, _ := url.Parse("https://asset-server.example.com")
	const wantTtl = int64(3600)
	apiConf := config.AdNormalizerConfig{
		AdServerUrl:    *adserverUrl,
		AssetServerUrl: *assetServerUrl,
		KeyField:       "url",
		KeyRegex:       "[^a-zA-Z0-9]",
		InFlightTtl:    int(wantTtl),
	}
	api := NewAPI(
		storeStub,
		apiConf,
		&EncoreHandlerStub{},
		&http.Client{},
		storeStub.kpiReport,
	)

	creative := structure.ManifestAsset{
		CreativeId:        "ttl-creative-id",
		MasterPlaylistUrl: "http://example.com/ad.mp4",
	}
	api.dispatchJobs(map[string]structure.ManifestAsset{creative.CreativeId: creative})

	// Give the goroutines a moment to complete.
	time.Sleep(100 * time.Millisecond)

	// Marker must be written exactly once.
	is.Equal(storeStub.sets, 1)

	// And it must carry the in-flight TTL, not be persisted forever.
	gotTtl, ok := storeStub.lastTtls[creative.CreativeId]
	is.True(ok)
	is.Equal(gotTtl, wantTtl)
}

func setupApi() (*API, *httptest.Server, *StoreStub, *EncoreHandlerStub) {
	storeStub := &StoreStub{
		mockStore: make(map[string]structure.TranscodeInfo),
		kpis:      normalizerMetrics.NormalizerMetrics{},
	}

	testServer := setupTestServer()

	encoreHandler := &EncoreHandlerStub{}
	adserverUrl, _ := url.Parse(testServer.URL)
	assetServerUrl, _ := url.Parse("https://asset-server.example.com")
	apiConf := config.AdNormalizerConfig{
		AdServerUrl:    *adserverUrl,
		AssetServerUrl: *assetServerUrl,
		KeyField:       "url",
		KeyRegex:       "[^a-zA-Z0-9]",
		KpiPostUrl:     "http://kpi-post.example.com/metrics",
		InFlightTtl:    3600,
	}
	// Initialize the API with the mock store.
	// Default checkAssetExists to always-true so existing tests don't make real
	// HTTP calls to external asset URLs. Individual tests that exercise the
	// existence-check logic override this field directly.
	api := NewAPI(
		storeStub,
		apiConf,
		encoreHandler,
		&http.Client{},
		storeStub.kpiReport,
	)
	api.checkAssetExists = func(assetUrl string) bool { return true }
	return api, testServer, storeStub, encoreHandler
}

func TestReplaceVast(t *testing.T) {
	is := is.New(t)
	api, ts, storeStub, encoreHandler := setupApi()
	defer ts.Close()
	// Populate the store with one ad
	adKey := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		ts.URL,
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("accept", "application/xml")
	// make sure we request a VAST response
	qps := vastReq.URL.Query()
	newUrl := strings.Replace(ts.URL, "127", "128", 1)
	parsedUrl, err := url.Parse(newUrl)
	is.NoErr(err)
	api.adServerUrl = *parsedUrl
	qps.Set("requestType", "vast")
	qps.Set("subDomain", "127")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vastRes, err := vmap.DecodeVast(responseBody)
	is.NoErr(err)
	is.Equal(len(vastRes.Ad), 1)
	mediaFile := vastRes.Ad[0].InLine.Creatives[0].Linear.MediaFiles[0]
	is.Equal(mediaFile.MediaType, "application/x-mpegURL")
	is.Equal(mediaFile.Text, "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8")
	is.Equal(mediaFile.Width, 718)
	is.Equal(mediaFile.Height, 404)

	realUrl, _ := url.Parse(ts.URL)
	api.adServerUrl = *realUrl // Reset to original URL

	is.Equal(storeStub.kpis.BrokenAds, 0)
	is.Equal(storeStub.kpis.IngestedAds, 1)
	is.Equal(storeStub.kpis.ServedAds, 1)

	encoreHandler.reset()
	storeStub.reset()
}

func TestReplaceVastWithDashPrefer(t *testing.T) {
	is := is.New(t)
	api, ts, storeStub, encoreHandler := setupApi()
	defer ts.Close()
	adKey := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		ts.URL,
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("accept", "application/xml")
	vastReq.Header.Set("prefer", "manifest-format=dash")
	qps := vastReq.URL.Query()
	qps.Set("requestType", "vast")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Preference-Applied"), "manifest-format=dash")
	is.Equal(recorder.Result().Header.Get("Vary"), "Prefer")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vastRes, err := vmap.DecodeVast(responseBody)
	is.NoErr(err)
	is.Equal(len(vastRes.Ad), 1)
	mediaFile := vastRes.Ad[0].InLine.Creatives[0].Linear.MediaFiles[0]
	is.Equal(mediaFile.MediaType, "application/dash+xml")
	is.Equal(mediaFile.Text, "https://testcontent.eyevinn.technology/ads/manifest.mpd")

	encoreHandler.reset()
	storeStub.reset()
}

func TestRequestedManifestFormat(t *testing.T) {
	is := is.New(t)
	request, err := http.NewRequest("GET", "/", nil)
	is.NoErr(err)
	manifestFormat, applied := requestedManifestFormat(request)
	is.Equal(manifestFormat, structure.ManifestFormatHLS)
	is.Equal(applied, false)

	request.Header.Set("Prefer", "manifest-format=vnd.apple.mpegurl")
	manifestFormat, applied = requestedManifestFormat(request)
	is.Equal(manifestFormat, structure.ManifestFormatHLS)
	is.Equal(applied, true)

	request.Header.Set("Prefer", "respond-async, manifest-format=application/dash+xml")
	manifestFormat, applied = requestedManifestFormat(request)
	is.Equal(manifestFormat, structure.ManifestFormatDASH)
	is.Equal(applied, true)

	request.Header.Set("Prefer", "manifest-format=unsupported")
	manifestFormat, applied = requestedManifestFormat(request)
	is.Equal(manifestFormat, structure.ManifestFormatHLS)
	is.Equal(applied, false)
}

func TestReplaceVastWithBlacklisted(t *testing.T) {
	is := is.New(t)
	api, ts, storeStub, encoreHandler := setupApi()
	defer ts.Close()
	// Populate the store with one ad
	adKey := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		ts.URL,
		nil,
	)
	is.NoErr(err)
	_ = storeStub.BlackList("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("accept", "application/xml")
	// make sure we request a VAST response
	qps := vastReq.URL.Query()
	is.NoErr(err)
	qps.Set("requestType", "vast")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vastRes, err := vmap.DecodeVast(responseBody)
	is.NoErr(err)
	is.Equal(len(vastRes.Ad), 0) // since the ad is blacklisted, we should not get any ads back

	is.Equal(storeStub.kpis.BrokenAds, 1)
	is.Equal(storeStub.kpis.IngestedAds, 1)
	is.Equal(storeStub.kpis.ServedAds, 0)

	encoreHandler.reset()
	storeStub.reset()

}

func TestReplaceVastWithFiller(t *testing.T) {
	is := is.New(t)

	api, ts, storeStub, encoreHandler := setupApi()
	defer ts.Close()
	adKey := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	// add a filler
	fillerInfo := structure.TranscodeInfo{
		Url:         "http://example.com/video.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	fillerKey := util.HashCreativeUrl("http://example.com/video.mp4")
	_ = storeStub.Set(fillerKey, fillerInfo)

	vastReq, err := http.NewRequest(
		"GET",
		ts.URL,
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("accept", "application/xml")
	qps := vastReq.URL.Query()
	qps.Set("requestType", "vast")
	qps.Set("filler", "http://example.com/video.mp4")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vastRes, err := vmap.DecodeVast(responseBody)
	is.NoErr(err)
	is.Equal(len(vastRes.Ad), 2)
	mediaFile := vastRes.Ad[0].InLine.Creatives[0].Linear.MediaFiles[0]
	is.Equal(mediaFile.MediaType, "application/x-mpegURL")
	is.Equal(mediaFile.Text, "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8")
	is.Equal(mediaFile.Width, 718)
	is.Equal(mediaFile.Height, 404)

	filler := vastRes.Ad[1]
	is.Equal(filler.Id, "NORMALIZER_FILLER")

	is.Equal(storeStub.kpis.BrokenAds, 0)
	is.Equal(storeStub.kpis.IngestedAds, 1)
	is.Equal(storeStub.kpis.ServedAds, 2)

	encoreHandler.reset()
	storeStub.reset()
}

func TestGetAssetList(t *testing.T) {
	is := is.New(t)
	// Populate the store with one ad
	api, ts, storeStub, encoreHandler := setupApi()
	defer ts.Close()
	adKey := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		ts.URL,
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("Accept", "application/json")
	// make sure we request a VAST response
	qps := vastReq.URL.Query()
	qps.Set("requestType", "vast")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/json")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	var assetList []structure.AssetDescription
	err = json.Unmarshal(responseBody, &assetList)
	is.NoErr(err)
	is.Equal(len(assetList), 1)
	is.Equal(assetList[0].Uri, transcodeInfo.Url)
	is.Equal(assetList[0].Duration, 10.25)

	encoreHandler.reset()
	storeStub.reset()
}

func TestGetAssetListWithDashPrefer(t *testing.T) {
	is := is.New(t)
	api, ts, storeStub, encoreHandler := setupApi()
	defer ts.Close()
	adKey := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		ts.URL,
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("Accept", "application/json")
	vastReq.Header.Set("Prefer", "manifest-format=application/dash+xml")
	qps := vastReq.URL.Query()
	qps.Set("requestType", "vast")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/json")
	is.Equal(recorder.Result().Header.Get("Preference-Applied"), "manifest-format=dash")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	var assetList []structure.AssetDescription
	err = json.Unmarshal(responseBody, &assetList)
	is.NoErr(err)
	is.Equal(len(assetList), 1)
	is.Equal(assetList[0].Uri, "https://testcontent.eyevinn.technology/ads/manifest.mpd")
	is.Equal(assetList[0].Duration, 10.25)

	encoreHandler.reset()
	storeStub.reset()
}

func TestEmptyVmap(t *testing.T) {
	is := is.New(t)
	api, ts, storeStub, encoreHandler := setupApi()
	defer ts.Close()
	vmapReq, err := http.NewRequest(
		"GET",
		ts.URL+"/vmap",
		nil,
	)
	is.NoErr(err)
	vmapReq.Header.Set("User-Agent", "TestUserAgent")
	vmapReq.Header.Set("X-Forwarded-For", "123.123.123")
	vmapReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vmapReq.Header.Set("accept", "application/xml")
	qps := vmapReq.URL.Query()
	qps.Set("requestType", "vmap")
	qps.Set("empty", "true") // tell test server to return empty vmap
	vmapReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVmap(recorder, vmapReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vmapRes, err := vmap.DecodeVmap(responseBody)
	is.NoErr(err)
	is.Equal(len(vmapRes.AdBreaks), 1)
	firstBreak := vmapRes.AdBreaks[0]
	is.Equal(firstBreak.TimeOffset.Position, vmap.OffsetStart)
	is.Equal(firstBreak.BreakType, "linear")
	is.Equal(len(firstBreak.AdSource.VASTData.VAST.Ad), 0)

	encoreHandler.reset()
	storeStub.reset()
}

func TestReplaceVmap(t *testing.T) {
	is := is.New(t)
	f, err := os.Open("../test_data/testVmap.xml")
	defer func() {
		_ = f.Close()
	}()
	is.NoErr(err)

	api, ts, storeStub, encoreHandler := setupApi()
	defer ts.Close()
	// Populate the store with one ad
	adKey := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vmapReq, err := http.NewRequest(
		"GET",
		ts.URL+"/vmap",
		nil,
	)
	is.NoErr(err)
	vmapReq.Header.Set("User-Agent", "TestUserAgent")
	vmapReq.Header.Set("X-Forwarded-For", "123.123.123")
	vmapReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vmapReq.Header.Set("accept", "application/xml")
	qps := vmapReq.URL.Query()
	qps.Set("requestType", "vmap")
	vmapReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVmap(recorder, vmapReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vmapRes, err := vmap.DecodeVmap(responseBody)
	is.NoErr(err)
	is.Equal(len(vmapRes.AdBreaks), 1)

	firstBreak := vmapRes.AdBreaks[0]
	is.Equal(firstBreak.TimeOffset.Position, vmap.OffsetStart)
	is.Equal(firstBreak.BreakType, "linear")

	firstVast := firstBreak.AdSource.VASTData.VAST
	is.Equal(len(firstVast.Ad), 1)
	firstAd := firstVast.Ad[0]
	is.Equal(firstAd.Id, "POD_AD-ID_001")
	is.Equal(firstAd.Sequence, 1)
	is.Equal(len(firstAd.InLine.Creatives), 1)
	firstCreative := firstAd.InLine.Creatives[0]
	is.Equal(len(firstCreative.Linear.TrackingEvents), 5)
	is.Equal(len(firstCreative.Linear.ClickTracking), 1)
	is.Equal(firstCreative.Linear.Duration.Duration, 10*time.Second)
	mediaFile := firstCreative.Linear.MediaFiles[0]
	is.Equal(mediaFile.MediaType, "application/x-mpegURL")
	is.Equal(mediaFile.Text, "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8")
	is.Equal(mediaFile.Width, 718)
	is.Equal(mediaFile.Height, 404)

	is.Equal(storeStub.kpis.BrokenAds, 0)
	is.Equal(storeStub.kpis.IngestedAds, 1)
	is.Equal(storeStub.kpis.ServedAds, 1)

	encoreHandler.reset()
	storeStub.reset()
}

func TestReplaceVmapWithDashPrefer(t *testing.T) {
	is := is.New(t)
	api, ts, storeStub, encoreHandler := setupApi()
	defer ts.Close()
	adKey := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vmapReq, err := http.NewRequest(
		"GET",
		ts.URL+"/vmap",
		nil,
	)
	is.NoErr(err)
	vmapReq.Header.Set("accept", "application/xml")
	vmapReq.Header.Set("prefer", "manifest-format=dash")
	qps := vmapReq.URL.Query()
	qps.Set("requestType", "vmap")
	vmapReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVmap(recorder, vmapReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Preference-Applied"), "manifest-format=dash")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vmapRes, err := vmap.DecodeVmap(responseBody)
	is.NoErr(err)
	firstCreative := vmapRes.AdBreaks[0].AdSource.VASTData.VAST.Ad[0].InLine.Creatives[0]
	mediaFile := firstCreative.Linear.MediaFiles[0]
	is.Equal(mediaFile.MediaType, "application/dash+xml")
	is.Equal(mediaFile.Text, "https://testcontent.eyevinn.technology/ads/manifest.mpd")

	encoreHandler.reset()
	storeStub.reset()
}

func TestBlacklist(t *testing.T) {
	is := is.New(t)
	api, ts, storeStub, _ := setupApi()
	defer ts.Close()
	blacklistUrl := "https://adserver-assets.io/badfile.mp4"
	reqBody := blacklistRequest{
		MediaUrl: blacklistUrl,
	}
	serializedBody, err := json.Marshal(reqBody)
	is.NoErr(err)
	blacklistReq, err := http.NewRequest(
		"POST",
		ts.URL+"/blacklist/",
		bytes.NewBuffer(serializedBody),
	)
	is.NoErr(err)
	recorder := httptest.NewRecorder()
	api.HandleBlackList(recorder, blacklistReq)
	is.Equal(recorder.Result().StatusCode, http.StatusNoContent)
	is.Equal(len(storeStub.blacklist), 1)

	getBlacklistReq, err := http.NewRequest(
		"GET",
		ts.URL+"/blacklist/",
		nil,
	)
	is.NoErr(err)
	recorder = httptest.NewRecorder()
	api.HandleBlackList(recorder, getBlacklistReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/json")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	var blResponse blacklistResponse
	err = json.Unmarshal(responseBody, &blResponse)
	is.NoErr(err)
	is.Equal(len(blResponse.MediaUrls), 1)
	is.Equal(blResponse.MediaUrls[0], blacklistUrl)
	is.Equal(blResponse.Page, 0)
	is.Equal(blResponse.Size, 1)
	is.Equal(blResponse.TotalCount, int64(1))
	is.Equal(blResponse.Next, "")
	is.Equal(blResponse.Prev, "")

	//remove from blacklist
	unblacklistReq, err := http.NewRequest(
		"DELETE",
		ts.URL+"/blacklist/",
		bytes.NewBuffer(serializedBody),
	)
	is.NoErr(err)
	recorder = httptest.NewRecorder()
	api.HandleBlackList(recorder, unblacklistReq)
	is.Equal(recorder.Result().StatusCode, http.StatusNoContent)
	is.Equal(len(storeStub.blacklist), 0)
}

// TODO: Add test for status endpoint

func TestHandleJobList(t *testing.T) {
	is := is.New(t)
	api, ts, _, _ := setupApi()
	defer ts.Close()
	// Create test request
	req, err := http.NewRequest(http.MethodGet, "/status", nil)
	is.NoErr(err)

	// Create response recorder
	recorder := httptest.NewRecorder()

	// Call the handler
	api.HandleJobList(recorder, req)

	// Check the response
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/json")

	// Parse the response body
	var response statusResponse
	err = json.NewDecoder(recorder.Body).Decode(&response)
	is.NoErr(err)

	// Verify response contents
	is.Equal(response.Page, 0)
	is.Equal(len(response.Jobs), 10)
	is.Equal(response.Next, "/jobs?page=1&size=10")
	is.Equal(response.Prev, "")
	for i, job := range response.Jobs {
		expectedIndex := 9 - i // Since jobs are in descending order
		expectedUrl := "http://example.com/video" + strconv.Itoa(expectedIndex) + "/index.m3u8"
		expectedSource := "s3://fake-bucket/video" + strconv.Itoa(expectedIndex) + ".mp4"
		is.Equal(job.Url, expectedUrl)
		is.Equal(job.Status, "COMPLETED")
		is.Equal(job.Source, expectedSource)
		is.True(job.LastUpdate > 0)
	}
}

func TestHandleJobListInvalidPageParameter(t *testing.T) {
	is := is.New(t)
	api, ts, _, _ := setupApi()
	defer ts.Close()
	// Test with invalid page parameter
	req, err := http.NewRequest(http.MethodGet, "/status?page=invalid", nil)
	is.NoErr(err)

	recorder := httptest.NewRecorder()
	api.HandleJobList(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusBadRequest)

	body, err := io.ReadAll(recorder.Body)
	is.NoErr(err)
	is.True(strings.Contains(string(body), "Invalid page parameter"))
}

func TestHandleJobListInvalidSizeParameter(t *testing.T) {
	is := is.New(t)
	api, ts, _, _ := setupApi()
	defer ts.Close()
	// Test with invalid size parameter
	req, err := http.NewRequest(http.MethodGet, "/status?size=invalid", nil)
	is.NoErr(err)

	recorder := httptest.NewRecorder()
	api.HandleJobList(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusBadRequest)

	body, err := io.ReadAll(recorder.Body)
	is.NoErr(err)
	is.True(strings.Contains(string(body), "Invalid size parameter"))
}

func TestHandleJobListNegativePageParameter(t *testing.T) {
	is := is.New(t)
	api, ts, _, _ := setupApi()
	defer ts.Close()
	// Test with negative page parameter
	req, err := http.NewRequest(http.MethodGet, "/status?page=-1", nil)
	is.NoErr(err)

	recorder := httptest.NewRecorder()
	api.HandleJobList(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusBadRequest)

	body, err := io.ReadAll(recorder.Body)
	is.NoErr(err)
	is.True(strings.Contains(string(body), "Invalid page parameter"))
}

func TestHandleJobListInvalidSizeParameterZero(t *testing.T) {
	is := is.New(t)
	api, ts, _, _ := setupApi()
	defer ts.Close()
	// Test with size parameter as zero
	req, err := http.NewRequest(http.MethodGet, "/status?size=0", nil)
	is.NoErr(err)

	recorder := httptest.NewRecorder()
	api.HandleJobList(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusBadRequest)

	body, err := io.ReadAll(recorder.Body)
	is.NoErr(err)
	is.True(strings.Contains(string(body), "Invalid size parameter"))
}

func TestHandlePreIngestCreatives(t *testing.T) {
	is := is.New(t)
	api, ts, storeStub, _ := setupApi()
	defer ts.Close()

	adKey := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	err := storeStub.Set(adKey, structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	})
	is.NoErr(err)

	preIngestCreativeRequest := preIngestCreativeRequest{
		MediaUrls: []string{
			"https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4",
			"https://testcontent.eyevinn.technology/ads/new-ad.mp4",
		},
	}
	serializedBody, err := json.Marshal(preIngestCreativeRequest)
	is.NoErr(err)

	req, err := http.NewRequest(
		http.MethodPost,
		"/pre-ingest-creatives",
		bytes.NewBuffer(serializedBody),
	)
	is.NoErr(err)
	recorder := httptest.NewRecorder()
	api.HandlePreIngestCreatives(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)

	var response preIngestCreativeResponse
	err = json.Unmarshal(responseBody, &response)
	is.NoErr(err)

	is.Equal(response.NotYetProcessed, 1) // One creative is unknown and should be processed
}

// Creative ids are now a fixed-length hash of the URL, so a long URL can no
// longer produce an over-long Encore filename on its own (see
// util.HashCreativeUrl). The length guard in partitionCreatives is kept as a
// fallback regardless - e.g. for the default (non-"url") keyField mode, which
// keys off the raw UniversalAdId - so it's exercised directly here.
func TestPartitionCreativesTooLongCreativeId(t *testing.T) {
	is := is.New(t)
	api, ts, storeStub, _ := setupApi()
	defer ts.Close()

	longCreativeId := strings.Repeat("a", 250)
	offendingUrl := "https://testcontent.eyevinn.technology/ads/offending.mp4"
	okUrl := "https://testcontent.eyevinn.technology/ads/new-ad.mp4"

	creatives := map[string]structure.ManifestAsset{
		longCreativeId: {
			CreativeId:        longCreativeId,
			MasterPlaylistUrl: offendingUrl,
			Source:            offendingUrl,
		},
		"shortid": {
			CreativeId:        "shortid",
			MasterPlaylistUrl: okUrl,
			Source:            okUrl,
		},
	}

	found, missing, filteredOut := api.partitionCreatives(creatives, structure.ManifestFormatHLS)
	is.Equal(len(found), 0)
	is.Equal(len(missing), 1) // only the short id should be dispatched
	_, stillMissing := missing[longCreativeId]
	is.True(!stillMissing)
	is.Equal(filteredOut, 1)

	blacklisted, err := storeStub.InBlackList(offendingUrl)
	is.NoErr(err)
	is.True(blacklisted) // the offending media URL should be blacklisted

	notBlacklisted, err := storeStub.InBlackList(okUrl)
	is.NoErr(err)
	is.True(!notBlacklisted)
}

// TestPartitionCreativesAsset404EvictsCache verifies that when the cached
// master-playlist URL returns 404, the cache key is deleted and the creative
// is treated as missing (queued for re-transcoding).
func TestPartitionCreativesAsset404EvictsCache(t *testing.T) {
	is := is.New(t)

	api, ts, storeStub, _ := setupApi()
	defer ts.Close()

	// Inject a stub that simulates a 404 for the cached asset URL.
	api.checkAssetExists = func(assetUrl string) bool { return false }

	creativeId := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	cachedUrl := "https://asset-server.example.com/ads/alvedon-10s/index.m3u8"

	_ = storeStub.Set(creativeId, structure.TranscodeInfo{
		Url:    cachedUrl,
		Status: "COMPLETED",
		Source: "https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4",
	})

	creatives := map[string]structure.ManifestAsset{
		creativeId: {
			CreativeId:        creativeId,
			MasterPlaylistUrl: "https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4",
			Source:            "https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4",
		},
	}

	found, missing, filteredOut := api.partitionCreatives(creatives, structure.ManifestFormatHLS)

	is.Equal(len(found), 0)   // must not be served — asset is gone
	is.Equal(len(missing), 1) // must be re-queued for re-transcoding
	is.Equal(filteredOut, 0)
	is.Equal(storeStub.deletes, 1) // cache key must have been evicted

	_, stillInCache := storeStub.mockStore[creativeId]
	is.True(!stillInCache) // key must be gone from the store
}

// TestPartitionCreativesAssetTransientErrorPreservesCache verifies that a
// transient or non-404 error from the asset server does NOT evict the cache —
// only a definitive 404 should trigger eviction.
func TestPartitionCreativesAssetTransientErrorPreservesCache(t *testing.T) {
	is := is.New(t)

	api, ts, storeStub, _ := setupApi()
	defer ts.Close()

	// Inject a stub that simulates "alive" (non-404, e.g. 200 or network error
	// treated as assume-alive).
	api.checkAssetExists = func(assetUrl string) bool { return true }

	creativeId := util.HashCreativeUrl("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	cachedUrl := "https://asset-server.example.com/ads/alvedon-10s/index.m3u8"

	_ = storeStub.Set(creativeId, structure.TranscodeInfo{
		Url:    cachedUrl,
		Status: "COMPLETED",
		Source: "https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4",
	})

	creatives := map[string]structure.ManifestAsset{
		creativeId: {
			CreativeId:        creativeId,
			MasterPlaylistUrl: "https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4",
			Source:            "https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4",
		},
	}

	found, missing, filteredOut := api.partitionCreatives(creatives, structure.ManifestFormatHLS)

	is.Equal(len(found), 1) // must still be served — asset is alive
	is.Equal(len(missing), 0)
	is.Equal(filteredOut, 0)
	is.Equal(storeStub.deletes, 0) // cache must NOT have been evicted
}

func TestHandlePreIngestCreativesMethodNotAllowed(t *testing.T) {
	is := is.New(t)
	api, ts, _, _ := setupApi()
	defer ts.Close()

	// Test with GET method (should be rejected)
	req, err := http.NewRequest(
		http.MethodGet,
		"/pre-ingest-creatives",
		nil,
	)
	is.NoErr(err)

	recorder := httptest.NewRecorder()
	api.HandlePreIngestCreatives(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusMethodNotAllowed)

	body, err := io.ReadAll(recorder.Body)
	is.NoErr(err)
	is.True(strings.Contains(string(body), "Method not allowed"))
}

func setupTestServer() *httptest.Server {
	vastData, _ := os.ReadFile("../test_data/testVast.xml")
	vmapData, _ := os.ReadFile("../test_data/testVmap.xml")
	emptyVmapData, _ := os.ReadFile("../test_data/emptyVmap.xml")
	return httptest.NewServer(http.HandlerFunc(
		func(res http.ResponseWriter, req *http.Request) {
			switch req.URL.Query().Get("requestType") {
			case "vast":
				time.Sleep(time.Millisecond * 10)
				res.Header().Set("Content-Type", "application/xml")
				if strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
					res.Header().Set("Content-Encoding", "gzip")
					res.WriteHeader(http.StatusOK)
					writer := gzip.NewWriter(res)
					defer func() {
						_ = writer.Close()
					}()
					_, _ = writer.Write(vastData)
				} else {
					res.WriteHeader(http.StatusOK)
					_, _ = res.Write(vastData)
				}
			case "vmap":
				time.Sleep(time.Millisecond * 10)
				res.Header().Set("Content-Type", "application/xml")
				res.WriteHeader(200)
				if req.URL.Query().Get("empty") == "true" {
					_, _ = res.Write(emptyVmapData)
				} else {
					_, _ = res.Write(vmapData)
				}
			default:
				res.WriteHeader(404)
			}
		}))
}
