package serve

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Eyevinn/VMAP/vmap"
	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/google/uuid"
	"github.com/matryer/is"
)

type StoreStub struct {
	mockStore map[string]structure.TranscodeInfo
	sets      int
	gets      int
	deletes   int
}

// Delete implements store.Store.
func (s *StoreStub) Delete(key string) error {
	delete(s.mockStore, key)
	s.deletes++
	return nil
}

func (s *StoreStub) Get(key string) (structure.TranscodeInfo, bool, error) {
	s.gets++
	if value, exists := s.mockStore[key]; exists {
		return value, true, nil
	}
	return structure.TranscodeInfo{}, false, nil
}

func (s *StoreStub) Set(key string, value structure.TranscodeInfo, ttl ...int64) error {
	s.sets++
	s.mockStore[key] = value
	return nil
}

func (s *StoreStub) reset() {
	s.mockStore = make(map[string]structure.TranscodeInfo)
	s.sets = 0
	s.gets = 0
	s.deletes = 0
}

func (s *StoreStub) EnqueuePackagingJob(queueName string, message structure.PackagingQueueMessage) error {
	// This is a stub, in a real implementation this would enqueue the job to a queue
	return nil
}

type EncoreHandlerStub struct {
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
		},
	}, nil
}

func (e *EncoreHandlerStub) reset() {
	logger.Info("Resetting EncoreHandlerStub")
	e.calls = 0
}

func (e *EncoreHandlerStub) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	logger.Info("EncoreHandlerStub.createJob called")
	newJob := structure.EncoreJob{}
	e.calls += 1
	return newJob, nil
}

var api *API
var testServer *httptest.Server
var encoreHandler *EncoreHandlerStub
var storeStub *StoreStub

func TestMain(m *testing.M) {
	storeStub = &StoreStub{
		mockStore: make(map[string]structure.TranscodeInfo),
	}

	testServer = setupTestServer()
	defer testServer.Close()
	encoreHandler = &EncoreHandlerStub{}
	adserverUrl, _ := url.Parse(testServer.URL)
	assetServerUrl, _ := url.Parse("https://asset-server.example.com")
	apiConf := config.AdNormalizerConfig{
		AdServerUrl:    *adserverUrl,
		AssetServerUrl: *assetServerUrl,
		KeyField:       "url",
		KeyRegex:       "[^a-zA-Z0-9]",
	}
	// Initialize the API with the mock store
	api = NewAPI(
		storeStub,
		apiConf,
		encoreHandler,
		&http.Client{}, // Use nil for the client in tests, or you can create a mock client
	)

	// Run the tests
	exitCode := m.Run()

	// Clean up if necessary
	os.Exit(exitCode)
}

func TestReplaceVast(t *testing.T) {
	is := is.New(t)
	// Populate the store with one ad
	re := regexp.MustCompile("[^a-zA-Z0-9]")
	adKey := re.ReplaceAllString("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4", "")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		testServer.URL,
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("accept", "application/xml")
	// make sure we request a VAST response
	qps := vastReq.URL.Query()
	newUrl := strings.Replace(testServer.URL, "127", "128", 1)
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

	realUrl, _ := url.Parse(testServer.URL)
	api.adServerUrl = *realUrl // Reset to original URL

	encoreHandler.reset()
	storeStub.reset()
}

func TestGetAssetList(t *testing.T) {
	is := is.New(t)
	// Populate the store with one ad
	re := regexp.MustCompile("[^a-zA-Z0-9]")
	adKey := re.ReplaceAllString("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4", "")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		testServer.URL,
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

func TestReplaceVmap(t *testing.T) {
	is := is.New(t)
	f, err := os.Open("../test_data/testVmap.xml")
	defer func() {
		_ = f.Close()
	}()
	is.NoErr(err)

	// Populate the store with one ad
	re := regexp.MustCompile("[^a-zA-Z0-9]")
	adKey := re.ReplaceAllString("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4", "")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vmapReq, err := http.NewRequest(
		"GET",
		testServer.URL+"/vmap",
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
	encoreHandler.reset()
	storeStub.reset()
}

func setupTestServer() *httptest.Server {
	vastData, _ := os.ReadFile("../test_data/testVast.xml")
	vmapData, _ := os.ReadFile("../test_data/testVmap.xml")
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
				_, _ = res.Write(vmapData)
			default:
				res.WriteHeader(404)
			}
		}))
}
